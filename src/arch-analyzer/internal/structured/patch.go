package structured

import (
	"encoding/json"
	"fmt"
	"strings"
)

func applyPatches(facts []Fact, patches []PatchSet, policies []AssemblyPolicy, fingerprint, versionScope string) ([]Fact, []ProposalDisposition, error) {
	result := append([]Fact{}, facts...)
	byID := indexFacts(result)
	seenPatchIDs := map[string]bool{}
	seenTargets := map[string]string{}
	var dispositions []ProposalDisposition
	policyByPatch, err := indexPolicies(policies)
	if err != nil {
		return nil, nil, err
	}

	for _, patch := range patches {
		if patch.SchemaVersion != PatchSchemaVersion {
			return nil, nil, fmt.Errorf("patch %q has unsupported schema_version %q", patch.PatchID, patch.SchemaVersion)
		}
		if strings.TrimSpace(patch.PatchID) == "" || seenPatchIDs[patch.PatchID] {
			return nil, nil, fmt.Errorf("patch_id must be non-empty and unique: %q", patch.PatchID)
		}
		if patch.Operations == nil {
			return nil, nil, fmt.Errorf("patch %q operations must be an explicit array", patch.PatchID)
		}
		seenPatchIDs[patch.PatchID] = true
		if patch.BundleFingerprint != fingerprint {
			return nil, nil, fmt.Errorf("patch %q bundle_fingerprint mismatch: got %q, want %q", patch.PatchID, patch.BundleFingerprint, fingerprint)
		}
		proposalFingerprint, err := ProposalFingerprint(patch)
		if err != nil {
			return nil, nil, fmt.Errorf("patch %q: %w", patch.PatchID, err)
		}
		policy, hasPolicy := policyByPatch[patch.PatchID]
		if len(patch.Operations) == 0 && !hasPolicy {
			continue
		}
		if !hasPolicy {
			return nil, nil, fmt.Errorf("patch %q has operations but no trusted assembly policy", patch.PatchID)
		}
		if err := validatePolicyBinding(policy, patch, proposalFingerprint, fingerprint, versionScope); err != nil {
			return nil, nil, err
		}
		allowed := map[string]bool{}
		if len(policy.Authority.AllowedFactTypes) == 0 {
			return nil, nil, fmt.Errorf("patch %q authority requires at least one allowed fact type", patch.PatchID)
		}
		for _, factType := range policy.Authority.AllowedFactTypes {
			if _, ok := descriptorByType(factType); !ok {
				return nil, nil, fmt.Errorf("patch %q authority names unsupported fact type %q", patch.PatchID, factType)
			}
			if allowed[factType] {
				return nil, nil, fmt.Errorf("patch %q authority repeats allowed fact type %q", patch.PatchID, factType)
			}
			allowed[factType] = true
		}
		decisions, err := decisionsFor(policy, patch)
		if err != nil {
			return nil, nil, err
		}
		seenOperationIDs := map[string]bool{}
		for _, operation := range patch.Operations {
			if strings.TrimSpace(operation.OperationID) == "" || seenOperationIDs[operation.OperationID] {
				return nil, nil, fmt.Errorf("patch %q operation_id must be non-empty and unique: %q", patch.PatchID, operation.OperationID)
			}
			seenOperationIDs[operation.OperationID] = true
			descriptor, ok := descriptorByType(operation.FactType)
			if !ok {
				return nil, nil, fmt.Errorf("patch %q operation %q has unsupported fact type %q", patch.PatchID, operation.OperationID, operation.FactType)
			}
			if !allowed[operation.FactType] {
				return nil, nil, fmt.Errorf("patch %q operation %q is not authorized for fact type %q", patch.PatchID, operation.OperationID, operation.FactType)
			}
			decision := decisions[operation.OperationID]
			if operation.Action != "add" && operation.Action != "update" && operation.Action != "delete" {
				return nil, nil, fmt.Errorf("patch %q operation %q has invalid action %q", patch.PatchID, operation.OperationID, operation.Action)
			}
			if operation.Action != "add" && operation.Key != "" {
				return nil, nil, fmt.Errorf("patch %q operation %q may set key only for add", patch.PatchID, operation.OperationID)
			}
			keyed := descriptor.Shape == shapeMap || descriptor.Shape == shapeMapArray
			if operation.Action == "add" && keyed != (operation.Key != "") {
				return nil, nil, fmt.Errorf("patch %q add operation %q key does not match fact type %q", patch.PatchID, operation.OperationID, operation.FactType)
			}
			if operation.Action == "add" && operation.TargetFactID != "" {
				return nil, nil, fmt.Errorf("patch %q add operation %q must not set target_fact_id", patch.PatchID, operation.OperationID)
			}
			if operation.Action != "add" && operation.TargetFactID == "" {
				return nil, nil, fmt.Errorf("patch %q %s operation %q requires target_fact_id", patch.PatchID, operation.Action, operation.OperationID)
			}
			if operation.Action == "delete" && len(operation.Value) != 0 && string(operation.Value) != "null" {
				return nil, nil, fmt.Errorf("patch %q delete operation %q must not carry value", patch.PatchID, operation.OperationID)
			}
			if operation.Action != "delete" {
				if _, err := validatePatchValue(descriptor, operation.Value); err != nil {
					return nil, nil, fmt.Errorf("patch %q %s operation %q: %w", patch.PatchID, operation.Action, operation.OperationID, err)
				}
			}
			if decision.Decision == "accept" && operation.Action != "add" &&
				(policy.Origin.ClaimClass == "planned" || policy.Origin.ClaimClass == "support") {
				return nil, nil, fmt.Errorf("patch %q may only add %s claims; it cannot mutate implementation facts", patch.PatchID, policy.Origin.ClaimClass)
			}
			if strings.TrimSpace(operation.Reason) == "" {
				return nil, nil, fmt.Errorf("patch %q operation %q requires a reason", patch.PatchID, operation.OperationID)
			}
			if err := validateEvidence(operation.Evidence); err != nil {
				return nil, nil, fmt.Errorf("patch %q operation %q evidence: %w", patch.PatchID, operation.OperationID, err)
			}
			if operation.Evidence == nil {
				return nil, nil, fmt.Errorf("patch %q operation %q evidence must be an explicit array", patch.PatchID, operation.OperationID)
			}
			if decision.Decision == "accept" && len(operation.Evidence) == 0 {
				return nil, nil, fmt.Errorf("patch %q accepted operation %q requires evidence", patch.PatchID, operation.OperationID)
			}

			disposition := ProposalDisposition{
				PatchID: patch.PatchID, ProposalFingerprint: proposalFingerprint,
				BundleFingerprint: patch.BundleFingerprint, OperationID: operation.OperationID,
				Action: operation.Action, FactType: operation.FactType,
				OriginKind: policy.Origin.Kind, OriginID: policy.Origin.ID, ClaimClass: policy.Origin.ClaimClass,
				Status: decision.Decision + "ed", TargetFactID: operation.TargetFactID,
				Evidence: append([]EvidenceRef{}, operation.Evidence...), ProposalReason: operation.Reason,
				PolicyID: policy.PolicyID, AuthorizedBy: policy.Authority.Actor,
				AllowedFactTypes: append([]string{}, policy.Authority.AllowedFactTypes...), DecisionID: decision.DecisionID,
				DecidedBy: decision.DecidedBy, DecisionReason: decision.Reason,
			}
			if decision.Decision == "reject" {
				if operation.TargetFactID != "" {
					if _, exists := byID[operation.TargetFactID]; !exists {
						return nil, nil, fmt.Errorf("patch %q rejected operation %q references missing target %q", patch.PatchID, operation.OperationID, operation.TargetFactID)
					}
				}
				dispositions = append(dispositions, disposition)
				continue
			}

			switch operation.Action {
			case "add":
				canonical, err := validatePatchValue(descriptor, operation.Value)
				if err != nil {
					return nil, nil, fmt.Errorf("patch %q add operation %q: %w", patch.PatchID, operation.OperationID, err)
				}
				id := factID(operation.FactType, operation.Key, canonical)
				if _, exists := byID[id]; exists {
					return nil, nil, fmt.Errorf("patch %q add operation %q duplicates fact %q", patch.PatchID, operation.OperationID, id)
				}
				for _, existing := range result {
					if existing.Type != operation.FactType {
						continue
					}
					if descriptor.Shape == shapeSingleton || descriptor.Shape == shapeNestedSingleton ||
						(keyed && existing.Key == operation.Key) {
						return nil, nil, fmt.Errorf("patch %q add operation %q would duplicate singleton/keyed fact type %q", patch.PatchID, operation.OperationID, operation.FactType)
					}
				}
				evidence := append(make([]EvidenceRef, 0, len(operation.Evidence)), operation.Evidence...)
				newFact := Fact{
					ID: id, Type: operation.FactType, Key: operation.Key,
					Ordinal: nextOrdinal(result, operation.FactType, operation.Key), Value: canonical,
					InputPointer: "/patches/" + escapePointer(patch.PatchID) + "/" + escapePointer(operation.OperationID),
					Evidence:     evidence, Uncertainty: uncertaintyFrom(canonical),
					Authority: FactAuthority{Kind: policy.Origin.Kind, Origin: policy.Origin.ID, ClaimClass: policy.Origin.ClaimClass},
				}
				result = append(result, newFact)
				byID[id] = len(result) - 1
				disposition.ResultingFactID = id
			case "update":
				index, target, err := targetFor(operation, byID, result, descriptor, seenTargets, patch.PatchID)
				if err != nil {
					return nil, nil, err
				}
				canonical, err := validatePatchValue(descriptor, operation.Value)
				if err != nil {
					return nil, nil, fmt.Errorf("patch %q update operation %q: %w", patch.PatchID, operation.OperationID, err)
				}
				updated := target
				updated.Value = canonical
				updated.InputPointer = "/patches/" + escapePointer(patch.PatchID) + "/" + escapePointer(operation.OperationID)
				updated.Evidence = append(make([]EvidenceRef, 0, len(operation.Evidence)), operation.Evidence...)
				updated.Uncertainty = uncertaintyFrom(canonical)
				updated.Authority = FactAuthority{Kind: policy.Origin.Kind, Origin: policy.Origin.ID, ClaimClass: policy.Origin.ClaimClass}
				updated.ID = factID(updated.Type, updated.Key, canonical)
				if otherIndex, exists := byID[updated.ID]; exists && otherIndex != index {
					return nil, nil, fmt.Errorf("patch %q update operation %q collides with fact %q", patch.PatchID, operation.OperationID, updated.ID)
				}
				delete(byID, target.ID)
				result[index] = updated
				byID[updated.ID] = index
				disposition.ResultingFactID = updated.ID
			case "delete":
				index, target, err := targetFor(operation, byID, result, descriptor, seenTargets, patch.PatchID)
				if err != nil {
					return nil, nil, err
				}
				result = append(result[:index], result[index+1:]...)
				delete(byID, target.ID)
				byID = indexFacts(result)
			}
			dispositions = append(dispositions, disposition)
		}
	}
	for patchID := range policyByPatch {
		if !seenPatchIDs[patchID] {
			return nil, nil, fmt.Errorf("trusted assembly policy references missing patch %q", patchID)
		}
	}
	return result, dispositions, nil
}

func indexPolicies(policies []AssemblyPolicy) (map[string]AssemblyPolicy, error) {
	result := make(map[string]AssemblyPolicy, len(policies))
	seenIDs := map[string]bool{}
	for _, policy := range policies {
		if policy.SchemaVersion != PolicySchemaVersion {
			return nil, fmt.Errorf("assembly policy %q has unsupported schema_version %q", policy.PolicyID, policy.SchemaVersion)
		}
		if strings.TrimSpace(policy.PolicyID) == "" || seenIDs[policy.PolicyID] {
			return nil, fmt.Errorf("assembly policy_id must be non-empty and unique: %q", policy.PolicyID)
		}
		seenIDs[policy.PolicyID] = true
		if strings.TrimSpace(policy.PatchID) == "" {
			return nil, fmt.Errorf("assembly policy %q requires patch_id", policy.PolicyID)
		}
		if _, duplicate := result[policy.PatchID]; duplicate {
			return nil, fmt.Errorf("multiple trusted assembly policies reference patch %q", policy.PatchID)
		}
		result[policy.PatchID] = policy
	}
	return result, nil
}

func validatePolicyBinding(policy AssemblyPolicy, patch PatchSet, proposalFingerprint, bundleFingerprint, versionScope string) error {
	if strings.EqualFold(strings.TrimSpace(versionScope), "unknown") {
		return fmt.Errorf("patch %q cannot be authorized for unknown version scope", patch.PatchID)
	}
	if policy.ProposalFingerprint != proposalFingerprint {
		return fmt.Errorf("assembly policy %q proposal_fingerprint mismatch for patch %q", policy.PolicyID, patch.PatchID)
	}
	if policy.BundleFingerprint != bundleFingerprint || policy.BundleFingerprint != patch.BundleFingerprint {
		return fmt.Errorf("assembly policy %q bundle_fingerprint mismatch for patch %q", policy.PolicyID, patch.PatchID)
	}
	if strings.TrimSpace(policy.VersionScope) == "" || strings.TrimSpace(policy.VersionScope) != policy.VersionScope ||
		strings.EqualFold(policy.VersionScope, "unknown") || policy.VersionScope != versionScope {
		return fmt.Errorf("assembly policy %q version_scope %q does not authorize accepted scope %q", policy.PolicyID, policy.VersionScope, versionScope)
	}
	if policy.Origin.Kind != "model" && policy.Origin.Kind != "human-correction" && policy.Origin.Kind != "overlay" {
		return fmt.Errorf("assembly policy %q has invalid origin kind %q", policy.PolicyID, policy.Origin.Kind)
	}
	if policy.Origin.ClaimClass != "implementation" && policy.Origin.ClaimClass != "support" &&
		policy.Origin.ClaimClass != "planned" && policy.Origin.ClaimClass != "correction" {
		return fmt.Errorf("assembly policy %q has invalid origin claim_class %q", policy.PolicyID, policy.Origin.ClaimClass)
	}
	if strings.TrimSpace(policy.Origin.ID) == "" || strings.TrimSpace(policy.Authority.Actor) == "" {
		return fmt.Errorf("assembly policy %q requires attributed origin.id and authority.actor", policy.PolicyID)
	}
	if policy.Decisions == nil {
		return fmt.Errorf("assembly policy %q decisions must be an explicit array", policy.PolicyID)
	}
	return nil
}

func decisionsFor(policy AssemblyPolicy, patch PatchSet) (map[string]AssemblyDecision, error) {
	operations := make(map[string]bool, len(patch.Operations))
	for _, operation := range patch.Operations {
		operations[operation.OperationID] = true
	}
	result := make(map[string]AssemblyDecision, len(policy.Decisions))
	seenDecisionIDs := map[string]bool{}
	for _, decision := range policy.Decisions {
		if strings.TrimSpace(decision.DecisionID) == "" || seenDecisionIDs[decision.DecisionID] {
			return nil, fmt.Errorf("assembly policy %q decision_id must be non-empty and unique: %q", policy.PolicyID, decision.DecisionID)
		}
		seenDecisionIDs[decision.DecisionID] = true
		if !operations[decision.OperationID] {
			return nil, fmt.Errorf("assembly policy %q decision %q references missing operation %q", policy.PolicyID, decision.DecisionID, decision.OperationID)
		}
		if _, duplicate := result[decision.OperationID]; duplicate {
			return nil, fmt.Errorf("assembly policy %q has multiple decisions for operation %q", policy.PolicyID, decision.OperationID)
		}
		if decision.Decision != "accept" && decision.Decision != "reject" {
			return nil, fmt.Errorf("assembly policy %q decision %q has invalid decision %q", policy.PolicyID, decision.DecisionID, decision.Decision)
		}
		if strings.TrimSpace(decision.DecidedBy) == "" || strings.TrimSpace(decision.Reason) == "" {
			return nil, fmt.Errorf("assembly policy %q decision %q requires decided_by and reason", policy.PolicyID, decision.DecisionID)
		}
		result[decision.OperationID] = decision
	}
	for _, operation := range patch.Operations {
		if _, ok := result[operation.OperationID]; !ok {
			return nil, fmt.Errorf("patch %q operation %q has no trusted decision", patch.PatchID, operation.OperationID)
		}
	}
	return result, nil
}

func validatePatchValue(descriptor factDescriptor, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("%s operation requires a typed value", descriptor.Type)
	}
	canonical, err := canonicalValue(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON value: %w", err)
	}
	if err := validateFactValue(descriptor, canonical); err != nil {
		return nil, fmt.Errorf("invalid typed value: %w", err)
	}
	return canonical, nil
}

func targetFor(operation PatchOperation, byID map[string]int, facts []Fact, descriptor factDescriptor, seenTargets map[string]string, patchID string) (int, Fact, error) {
	if operation.TargetFactID == "" {
		return 0, Fact{}, fmt.Errorf("patch %q %s operation %q requires target_fact_id", patchID, operation.Action, operation.OperationID)
	}
	if previous, conflict := seenTargets[operation.TargetFactID]; conflict {
		return 0, Fact{}, fmt.Errorf("conflicting accepted operations %q and %q target %q", previous, operation.OperationID, operation.TargetFactID)
	}
	index, exists := byID[operation.TargetFactID]
	if !exists {
		return 0, Fact{}, fmt.Errorf("patch %q operation %q references missing target %q", patchID, operation.OperationID, operation.TargetFactID)
	}
	target := facts[index]
	if target.Type != descriptor.Type {
		return 0, Fact{}, fmt.Errorf("patch %q operation %q fact_type %q does not match target type %q", patchID, operation.OperationID, operation.FactType, target.Type)
	}
	seenTargets[operation.TargetFactID] = operation.OperationID
	return index, target, nil
}

func indexFacts(facts []Fact) map[string]int {
	result := make(map[string]int, len(facts))
	for index, fact := range facts {
		result[fact.ID] = index
	}
	return result
}

func nextOrdinal(facts []Fact, factType, key string) int {
	ordinal := 0
	for _, fact := range facts {
		if fact.Type == factType && fact.Key == key && fact.Ordinal >= ordinal {
			ordinal = fact.Ordinal + 1
		}
	}
	return ordinal
}

func validateEvidence(evidence []EvidenceRef) error {
	for _, reference := range evidence {
		if strings.TrimSpace(reference.Path) == "" || filepathInvalid(reference.Path) {
			return fmt.Errorf("invalid repository-relative path %q", reference.Path)
		}
		if err := validateEvidenceAtom(reference.Path); err != nil {
			return fmt.Errorf("invalid evidence path %q: %w", reference.Path, err)
		}
		if reference.StartLine < 0 || reference.EndLine < 0 ||
			(reference.EndLine > 0 && reference.StartLine == 0) ||
			(reference.StartLine > 0 && reference.EndLine < reference.StartLine) {
			return fmt.Errorf("invalid line range %d-%d for %q", reference.StartLine, reference.EndLine, reference.Path)
		}
		if strings.TrimSpace(reference.Revision) == "" {
			return fmt.Errorf("missing source revision for %q", reference.Path)
		}
		if err := validateEvidenceAtom(reference.Revision); err != nil {
			return fmt.Errorf("invalid evidence revision for %q: %w", reference.Path, err)
		}
	}
	return nil
}

func filepathInvalid(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	if strings.HasPrefix(path, "/") {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "" {
			return true
		}
	}
	return false
}
