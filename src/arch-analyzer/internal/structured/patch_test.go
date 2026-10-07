package structured

import (
	"encoding/json"
	"strings"
	"testing"
)

const patchAnalyzer = `{
  "component":"example","repo":"org/example","commit_sha":"abc","extracted_at":"2026-09-06T12:00:00Z","analyzer_version":"test","schema_version":"1",
  "services":[{"name":"api","source":"config/service.yaml:1-10","type":"ClusterIP","ports":[],"target_deployment":"api"}],
  "integration_points":[]
}`

const patchScope = "rhoai-test"

func TestModelProposalCannotSelfAuthorizeUpdateOrDelete(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	for _, operation := range []PatchOperation{
		{
			OperationID: "model-update", Action: "update", FactType: "service", TargetFactID: target.ID,
			Value:    json.RawMessage(`{"name":"api","source":"config/service.yaml:1-10","type":"NodePort","ports":[],"target_deployment":"api"}`),
			Evidence: []EvidenceRef{{Path: "config/service.yaml", StartLine: 1, EndLine: 10, Revision: "abc"}}, Reason: "Model proposes an update.",
		},
		{
			OperationID: "model-delete", Action: "delete", FactType: "service", TargetFactID: target.ID,
			Evidence: []EvidenceRef{{Path: "config/service.yaml", StartLine: 1, EndLine: 10, Revision: "abc"}}, Reason: "Model proposes a deletion.",
		},
	} {
		patch := validPatch(base, operation)
		_, err := Normalize(strings.NewReader(patchAnalyzer), Options{VersionScope: patchScope, Patches: []PatchSet{patch}})
		if err == nil || !strings.Contains(err.Error(), "no trusted assembly policy") {
			t.Fatalf("%s without trusted policy error=%v", operation.Action, err)
		}
	}

	_, err := DecodePatch(strings.NewReader(`{
	  "schema_version":"1.0.0","patch_id":"spoof","bundle_fingerprint":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
	  "origin":{"kind":"model","id":"self","claim_class":"implementation"},
	  "authority":{"actor":"model","allowed_fact_types":["service"]},
	  "operations":[{"operation_id":"delete","action":"delete","decision":"accept","fact_type":"service","target_fact_id":"fact:service:0000000000000000000000000000000000000000000000000000000000000000","evidence":[],"reason":"self approve"}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("self-authorizing proposal DecodePatch() error=%v", err)
	}
}

func TestTrustedHumanDecisionAppliesModelProposalAndPersistsProvenance(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	patch := validPatch(base, PatchOperation{
		OperationID: "change-service-type", Action: "update", FactType: "service", TargetFactID: target.ID,
		Value:    json.RawMessage(`{"name":"api","source":"config/service.yaml:1-10","type":"NodePort","ports":[],"target_deployment":"api"}`),
		Evidence: []EvidenceRef{{Path: "config/service.yaml", StartLine: 1, EndLine: 10, Revision: "abc"}},
		Reason:   "Model-proposed manifest-backed correction.",
	})
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "model", ID: "response-01", ClaimClass: "correction"}, "accept")
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{
		VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Dispositions) != 1 || document.Dispositions[0].Status != "accepted" {
		t.Fatalf("dispositions=%#v", document.Dispositions)
	}
	disposition := document.Dispositions[0]
	if disposition.PolicyID != policy.PolicyID || disposition.DecidedBy != "human-reviewer@example" ||
		disposition.ProposalFingerprint != policy.ProposalFingerprint || disposition.ClaimClass != "correction" {
		t.Fatalf("trusted decision provenance lost: %#v", disposition)
	}
	if disposition.TargetFactID == disposition.ResultingFactID {
		t.Fatal("typed update should retain target ID and record resulting value ID separately")
	}
	updated := factOfType(t, document, "service")
	if updated.InputPointer != "/patches/test-patch/change-service-type" {
		t.Fatalf("updated fact input_pointer=%q", updated.InputPointer)
	}
	if got := document.RenderingView.Services[0].Type; got != "NodePort" {
		t.Fatalf("rendering service type=%q", got)
	}
}

func TestAcceptedPatchValidationRejectsMissingDispositionAndEvidence(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	patch := validPatch(base, PatchOperation{
		OperationID: "change-service-type", Action: "update", FactType: "service", TargetFactID: target.ID,
		Value:    json.RawMessage(`{"name":"api","source":"config/service.yaml:1-10","type":"NodePort","ports":[],"target_deployment":"api"}`),
		Evidence: []EvidenceRef{{Path: "config/service.yaml", Revision: "abc"}}, Reason: "reviewed proposal",
	})
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "model", ID: "response-01", ClaimClass: "correction"}, "accept")
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{
		VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy},
	})
	if err != nil {
		t.Fatal(err)
	}

	missingDisposition := document
	missingDisposition.Dispositions = []ProposalDisposition{}
	if err := Validate(missingDisposition); err == nil || !strings.Contains(err.Error(), "has no proposal disposition") {
		t.Fatalf("missing disposition Validate() error=%v", err)
	}

	missingEvidence := document
	missingEvidence.Dispositions = append([]ProposalDisposition{}, document.Dispositions...)
	missingEvidence.Dispositions[0].Evidence = nil
	if err := Validate(missingEvidence); err == nil || !strings.Contains(err.Error(), "explicit evidence array") {
		t.Fatalf("missing disposition evidence Validate() error=%v", err)
	}
}

func TestAcceptedDeleteCannotLoseItsDisposition(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	patch := validPatch(base, PatchOperation{
		OperationID: "delete-service", Action: "delete", FactType: "service", TargetFactID: target.ID,
		Evidence: []EvidenceRef{{Path: "config/service.yaml", Revision: "abc"}}, Reason: "reviewed deletion proposal",
	})
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "model", ID: "response-delete", ClaimClass: "correction"}, "accept")
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{
		VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.RenderingView.Services) != 0 || len(document.Dispositions) != 1 {
		t.Fatalf("accepted delete was not applied with disposition: services=%#v dispositions=%#v", document.RenderingView.Services, document.Dispositions)
	}
	document.Dispositions = []ProposalDisposition{}
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "has no proposal disposition") {
		t.Fatalf("accepted delete with missing disposition Validate() error=%v", err)
	}
}

func TestTrustedHumanRejectionPreservesAnalyzerFact(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	patch := validPatch(base, PatchOperation{
		OperationID: "reject-change", Action: "delete", FactType: "service", TargetFactID: target.ID,
		Evidence: []EvidenceRef{}, Reason: "Model proposal under review.",
	})
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "model", ID: "response-02", ClaimClass: "implementation"}, "reject")
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{
		VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.RenderingView.Services) != 1 || document.Dispositions[0].Status != "rejected" {
		t.Fatalf("rejected patch changed accepted facts: %#v", document.Dispositions)
	}
}

func TestTypedPatchAllowsExplicitEmptyOperationsWithoutPolicy(t *testing.T) {
	base := patchBase(t)
	patch := validPatch(base, PatchOperation{})
	patch.Operations = []PatchOperation{}
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{Patches: []PatchSet{patch}})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Dispositions) != 0 || len(document.RenderingView.Services) != 1 {
		t.Fatalf("empty patch changed document: dispositions=%#v services=%#v", document.Dispositions, document.RenderingView.Services)
	}
}

func TestPlannedOverlayIsRetainedButNotRenderedAsImplementation(t *testing.T) {
	base := patchBase(t)
	patch := validPatch(base, PatchOperation{
		OperationID: "roadmap-integration", Action: "add", FactType: "integration_point",
		Value:    json.RawMessage(`{"component":"future-service","interaction_type":"REST","purpose":"Roadmap integration"}`),
		Evidence: []EvidenceRef{{Path: "overlays/praxis.yaml", StartLine: 4, EndLine: 6, Revision: "abc"}},
		Reason:   "Retain attributed roadmap intent without claiming implementation.",
	})
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "overlay", ID: "release-roadmap", ClaimClass: "planned"}, "accept")
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{
		VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.RenderingView.IntegrationPoints) != 0 {
		t.Fatalf("planned relationship rendered as implementation: %#v", document.RenderingView.IntegrationPoints)
	}
	fact := factOfType(t, document, "integration_point")
	if fact.Authority.ClaimClass != "planned" {
		t.Fatalf("planned provenance lost: %#v", fact.Authority)
	}
}

func TestTrustedPolicyRejectsStaleUnauthorizedMissingAndConflictingOperations(t *testing.T) {
	base := patchBase(t)
	target := factOfType(t, base, "service")
	operation := PatchOperation{
		OperationID: "op", Action: "update", FactType: "service", TargetFactID: target.ID,
		Value:    json.RawMessage(`{"name":"api","source":"config/service.yaml","type":"NodePort","ports":[],"target_deployment":"api"}`),
		Evidence: []EvidenceRef{{Path: "config/service.yaml", Revision: "abc"}}, Reason: "test proposal",
	}

	t.Run("unknown scope cannot authorize correction", func(t *testing.T) {
		patch := validPatch(base, operation)
		policy := validPolicy(t, base, patch, PatchOrigin{Kind: "human-correction", ID: "correction", ClaimClass: "correction"}, "accept")
		_, err := Normalize(strings.NewReader(patchAnalyzer), Options{Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy}})
		if err == nil || !strings.Contains(err.Error(), "unknown version scope") {
			t.Fatalf("Normalize() error=%v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*PatchSet, *AssemblyPolicy)
		want   string
	}{
		{"stale proposal binding", func(_ *PatchSet, p *AssemblyPolicy) { p.ProposalFingerprint = "sha256:" + strings.Repeat("0", 64) }, "proposal_fingerprint mismatch"},
		{"wrong scope", func(_ *PatchSet, p *AssemblyPolicy) { p.VersionScope = "rhoai-other" }, "does not authorize"},
		{"unauthorized", func(_ *PatchSet, p *AssemblyPolicy) { p.Authority.AllowedFactTypes = []string{"crd"} }, "not authorized"},
		{"missing decision", func(_ *PatchSet, p *AssemblyPolicy) { p.Decisions = []AssemblyDecision{} }, "has no trusted decision"},
		{"missing evidence", func(patch *PatchSet, p *AssemblyPolicy) {
			patch.Operations[0].Evidence = []EvidenceRef{}
			p.ProposalFingerprint, _ = ProposalFingerprint(*patch)
		}, "requires evidence"},
		{"evidence path line injection", func(patch *PatchSet, p *AssemblyPolicy) {
			patch.Operations[0].Evidence = []EvidenceRef{{Path: "config/service.yaml\n```", Revision: "abc"}}
			p.ProposalFingerprint, _ = ProposalFingerprint(*patch)
		}, "single line without control"},
		{"evidence revision line injection", func(patch *PatchSet, p *AssemblyPolicy) {
			patch.Operations[0].Evidence = []EvidenceRef{{Path: "config/service.yaml", Revision: "abc\n```"}}
			p.ProposalFingerprint, _ = ProposalFingerprint(*patch)
		}, "single line without control"},
		{"missing target", func(patch *PatchSet, p *AssemblyPolicy) {
			patch.Operations[0].TargetFactID = "fact:service:" + strings.Repeat("0", 64)
			p.ProposalFingerprint, _ = ProposalFingerprint(*patch)
		}, "missing target"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patch := validPatch(base, operation)
			policy := validPolicy(t, base, patch, PatchOrigin{Kind: "human-correction", ID: "reviewed-correction", ClaimClass: "correction"}, "accept")
			test.mutate(&patch, &policy)
			_, err := Normalize(strings.NewReader(patchAnalyzer), Options{VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize() error=%v, want %q", err, test.want)
			}
		})
	}

	patch := validPatch(base, operation)
	second := operation
	second.OperationID = "op-2"
	patch.Operations = append(patch.Operations, second)
	policy := validPolicy(t, base, patch, PatchOrigin{Kind: "human-correction", ID: "reviewed-correction", ClaimClass: "correction"}, "accept")
	policy.Decisions = append(policy.Decisions, AssemblyDecision{DecisionID: "decision-2", OperationID: "op-2", Decision: "accept", DecidedBy: "human-reviewer@example", Reason: "Explicitly approved."})
	if _, err := Normalize(strings.NewReader(patchAnalyzer), Options{VersionScope: patchScope, Patches: []PatchSet{patch}, AssemblyPolicies: []AssemblyPolicy{policy}}); err == nil || !strings.Contains(err.Error(), "conflicting accepted operations") {
		t.Fatalf("conflicting operations error=%v", err)
	}

	plannedPatch := validPatch(base, operation)
	plannedPolicy := validPolicy(t, base, plannedPatch, PatchOrigin{Kind: "overlay", ID: "roadmap", ClaimClass: "planned"}, "accept")
	if _, err := Normalize(strings.NewReader(patchAnalyzer), Options{VersionScope: patchScope, Patches: []PatchSet{plannedPatch}, AssemblyPolicies: []AssemblyPolicy{plannedPolicy}}); err == nil || !strings.Contains(err.Error(), "cannot mutate implementation facts") {
		t.Fatalf("planned update error=%v", err)
	}

	_, err := DecodePatch(strings.NewReader(`{"schema_version":1,"operations":[]}`))
	if err == nil || !strings.Contains(err.Error(), "ambiguous legacy patch") {
		t.Fatalf("DecodePatch() error=%v", err)
	}
}

func TestProposalFingerprintUsesCanonicalJSONBytes(t *testing.T) {
	left, err := DecodePatch(strings.NewReader(`{"schema_version":"1.0.0","patch_id":"p","bundle_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","operations":[{"operation_id":"o","action":"add","fact_type":"service","value":{"type":"ClusterIP","name":"api","ports":[],"source":"service.yaml","target_deployment":"api"},"evidence":[],"reason":"r"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	right, err := DecodePatch(strings.NewReader(`{
	  "operations": [{"reason":"r", "evidence":[], "value":{"target_deployment":"api","source":"service.yaml","ports":[],"name":"api","type":"ClusterIP"}, "fact_type":"service", "action":"add", "operation_id":"o"}],
	  "bundle_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "patch_id":"p", "schema_version":"1.0.0"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	leftFingerprint, err := ProposalFingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightFingerprint, err := ProposalFingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftFingerprint != rightFingerprint {
		t.Fatalf("canonical proposal fingerprints differ: %s != %s", leftFingerprint, rightFingerprint)
	}
}

func patchBase(t *testing.T) Document {
	t.Helper()
	document, err := Normalize(strings.NewReader(patchAnalyzer), Options{VersionScope: patchScope})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func validPatch(document Document, operation PatchOperation) PatchSet {
	return PatchSet{
		SchemaVersion: PatchSchemaVersion, PatchID: "test-patch",
		BundleFingerprint: document.AnalyzerInput.BundleFingerprint,
		Operations:        []PatchOperation{operation},
	}
}

func validPolicy(t *testing.T, document Document, patch PatchSet, origin PatchOrigin, decision string) AssemblyPolicy {
	t.Helper()
	fingerprint, err := ProposalFingerprint(patch)
	if err != nil {
		t.Fatal(err)
	}
	operationID := ""
	factType := "service"
	if len(patch.Operations) > 0 {
		operationID = patch.Operations[0].OperationID
		factType = patch.Operations[0].FactType
	}
	return AssemblyPolicy{
		SchemaVersion: PolicySchemaVersion, PolicyID: "human-review-policy", PatchID: patch.PatchID,
		ProposalFingerprint: fingerprint, BundleFingerprint: document.AnalyzerInput.BundleFingerprint,
		VersionScope: patchScope, Origin: origin,
		Authority: PatchAuthority{Actor: "human-reviewer@example", AllowedFactTypes: []string{factType}},
		Decisions: []AssemblyDecision{{DecisionID: "decision-1", OperationID: operationID, Decision: decision, DecidedBy: "human-reviewer@example", Reason: "Explicitly reviewed and decided."}},
	}
}

func factOfType(t *testing.T, document Document, factType string) Fact {
	t.Helper()
	for _, fact := range document.Facts {
		if fact.Type == factType {
			return fact
		}
	}
	t.Fatalf("missing fact type %q", factType)
	return Fact{}
}
