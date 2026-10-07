package structured

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/jctanner/arch-analyzer/internal/model"
	"github.com/jctanner/arch-analyzer/internal/normalize"
)

var factIDPattern = regexp.MustCompile(`^fact:[a-z][a-z0-9_]*:[0-9a-f]{64}(?::([2-9]|[1-9][0-9]+))?$`)
var fingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var (
	headingPattern       = regexp.MustCompile(`^#{1,6}(?:\s|$)`)
	orderedListPattern   = regexp.MustCompile(`^[0-9]{1,9}[.)](?:\s|$)`)
	thematicBreakPattern = regexp.MustCompile(`^(?:\*\s*){3,}$|^(?:-\s*){3,}$|^(?:_\s*){3,}$`)
	rawHTMLPattern       = regexp.MustCompile(`<(?:[!?]|/?[A-Za-z])`)
	linkPattern          = regexp.MustCompile(`!?\[[^]]*\]\s*(?:\(|\[|:)`)
)

type SectionDefinition struct {
	ID       string
	Title    string
	ParentID string
	Order    int
}

type TableDefinition struct {
	ID        string
	SectionID string
	Title     string
	Order     int
	Headers   []string
}

var sectionRegistry = []SectionDefinition{
	{ID: "aipcc-ecosystems-use", Title: "AIPCC Ecosystems Use", Order: 45},
	{ID: "sub-component-details", Title: "Sub-Component Details", Order: 46},
	{ID: "deployment-manifests", Title: "Deployment Manifests", Order: 65},
	{ID: "security.fips-compliance", Title: "FIPS Compliance", ParentID: "security", Order: 84},
	{ID: "security.build-hermeticity", Title: "Build Hermeticity", ParentID: "security", Order: 85},
	{ID: "multi-tenancy", Title: "Multi-Tenancy", Order: 86},
}

var tableRegistry = []TableDefinition{
	{ID: "aipcc.accelerator-build-variants", SectionID: "aipcc-ecosystems-use", Title: "Accelerator Build Variants", Order: 1, Headers: []string{"Variant", "Dockerfile", "Base Image", "Accelerator", "Version", "Architectures", "Status"}},
	{ID: "aipcc.package-index", SectionID: "aipcc-ecosystems-use", Title: "AIPCC Package Index", Order: 2, Headers: []string{"Scope", "Details"}},
	{ID: "aipcc.tooling", SectionID: "aipcc-ecosystems-use", Title: "AIPCC Tooling", Order: 3, Headers: []string{"Tool / Path", "Purpose", "Used By"}},
	{ID: "subcomponent.summary", SectionID: "sub-component-details", Order: 1, Headers: []string{"Component", "Dockerfile", "Intent", "Image", "Language", "Port"}},
	{ID: "subcomponent.api-routes", SectionID: "sub-component-details", Title: "API Routes", Order: 2, Headers: []string{"Component", "Path", "Method", "Upstream Service", "Auth", "Purpose"}},
	{ID: "subcomponent.upstream-dependencies", SectionID: "sub-component-details", Title: "Upstream Dependencies", Order: 3, Headers: []string{"Component", "Service", "Protocol", "Port", "Auth", "Purpose"}},
	{ID: "subcomponent.configuration", SectionID: "sub-component-details", Title: "Configuration", Order: 4, Headers: []string{"Component", "Name", "Type", "Default", "Purpose"}},
	{ID: "deployment.kustomize-structure", SectionID: "deployment-manifests", Title: "Kustomize Structure", Order: 1, Headers: []string{"Base / Overlay", "Path", "Purpose"}},
	{ID: "deployment.parameterization", SectionID: "deployment-manifests", Title: "Parameterization", Order: 2, Headers: []string{"Parameter", "Source", "Default", "Purpose"}},
	{ID: "deployment.distribution-variants", SectionID: "deployment-manifests", Title: "Distribution Variants", Order: 3, Headers: []string{"Variant", "Path", "Differences"}},
	{ID: "security.fips-build-time", SectionID: "security.fips-compliance", Title: "Build-Time FIPS (check-payload gate)", Order: 1, Headers: []string{"Aspect", "Value", "Source"}},
	{ID: "security.fips-application-crypto", SectionID: "security.fips-compliance", Title: "Application-Level Crypto", Order: 2, Headers: []string{"Aspect", "Value", "Source"}},
	{ID: "security.build-hermeticity", SectionID: "security.build-hermeticity", Order: 1, Headers: []string{"Layer", "Lock File", "Present", "Tool", "Source"}},
	{ID: "multi-tenancy.tenant-model", SectionID: "multi-tenancy", Title: "Tenant Model", Order: 1, Headers: []string{"Aspect", "Value", "Source"}},
	{ID: "multi-tenancy.isolation-mechanisms", SectionID: "multi-tenancy", Title: "Isolation Mechanisms", Order: 2, Headers: []string{"Dimension", "Mechanism", "Enforced By", "Gaps / Risks"}},
	{ID: "multi-tenancy.shared-services", SectionID: "multi-tenancy", Title: "Shared Services", Order: 3, Headers: []string{"Shared Service", "Tenant Boundary", "Isolation Mechanism"}},
}

func SectionRegistry() []SectionDefinition {
	return append([]SectionDefinition{}, sectionRegistry...)
}

func TableRegistry() []TableDefinition {
	result := make([]TableDefinition, len(tableRegistry))
	copy(result, tableRegistry)
	for index := range result {
		result[index].Headers = append([]string{}, result[index].Headers...)
	}
	return result
}

func tableDefinition(id string) (TableDefinition, bool) {
	for _, definition := range tableRegistry {
		if definition.ID == id {
			return definition, true
		}
	}
	return TableDefinition{}, false
}

func sectionDefinition(id string) (SectionDefinition, bool) {
	for _, definition := range sectionRegistry {
		if definition.ID == id {
			return definition, true
		}
	}
	return SectionDefinition{}, false
}

func Validate(document Document) error {
	if document.SchemaVersion != DocumentSchemaVersion && document.SchemaVersion != PublishedDocumentSchemaVersion {
		return fmt.Errorf("unsupported structured document schema_version %q", document.SchemaVersion)
	}
	if document.SchemaVersion == PublishedDocumentSchemaVersion && document.Publication == nil {
		return fmt.Errorf("published structured document requires publication bindings")
	}
	if document.SchemaVersion == DocumentSchemaVersion && document.Publication != nil {
		return fmt.Errorf("private structured document must not contain publication bindings")
	}
	if strings.TrimSpace(document.Identity.Component) == "" || strings.TrimSpace(document.Identity.SourceComponent) == "" {
		return fmt.Errorf("structured document identity requires component and source_component")
	}
	if strings.TrimSpace(document.Identity.VersionScope) == "" {
		return fmt.Errorf("structured document identity requires version_scope")
	}
	if document.Identity.Aliases == nil || document.Facts == nil || document.FactAccounting == nil ||
		document.Sections == nil || document.PatchInputs == nil || document.Dispositions == nil || document.Uncertainty == nil {
		return fmt.Errorf("structured document requires explicit array values; null or missing arrays are invalid")
	}
	if document.AssemblyInputs == nil {
		return fmt.Errorf("structured document requires explicit assembly_inputs")
	}
	if err := validateIntegrationStatus(document.Identity.IntegrationStatus); err != nil {
		return err
	}
	if document.Producers.NormalizerVersion != NormalizerVersion || document.Producers.RendererVersion != RendererVersion {
		return fmt.Errorf("unsupported producer versions: normalizer=%q renderer=%q", document.Producers.NormalizerVersion, document.Producers.RendererVersion)
	}
	if !fingerprintPattern.MatchString(document.AnalyzerInput.BundleFingerprint) {
		return fmt.Errorf("invalid analyzer bundle_fingerprint %q", document.AnalyzerInput.BundleFingerprint)
	}
	if document.AnalyzerInput.SchemaVersion != "1" {
		return fmt.Errorf("unsupported analyzer schema_version %q", document.AnalyzerInput.SchemaVersion)
	}
	if err := ValidateSections(document.Sections); err != nil {
		return err
	}
	if err := validateAssemblyInputs(*document.AssemblyInputs); err != nil {
		return err
	}
	if document.Reuse != nil {
		if err := validateReuseRecord(*document.Reuse, document); err != nil {
			return err
		}
	}
	if document.Publication != nil {
		if err := validatePublication(*document.Publication, document); err != nil {
			return err
		}
	}

	facts := map[string]Fact{}
	for _, fact := range document.Facts {
		if !factIDPattern.MatchString(fact.ID) {
			return fmt.Errorf("invalid fact ID %q", fact.ID)
		}
		if _, duplicate := facts[fact.ID]; duplicate {
			return fmt.Errorf("duplicate fact ID %q", fact.ID)
		}
		descriptor, known := descriptorByType(fact.Type)
		if !known {
			return fmt.Errorf("unsupported required fact type %q", fact.Type)
		}
		if fact.Ordinal < 0 || strings.TrimSpace(fact.InputPointer) == "" {
			return fmt.Errorf("fact %q has invalid ordinal or input_pointer", fact.ID)
		}
		if !strings.HasPrefix(fact.InputPointer, "/") {
			return fmt.Errorf("fact %q input_pointer must be a JSON pointer", fact.ID)
		}
		keyed := descriptor.Shape == shapeMap || descriptor.Shape == shapeMapArray
		if keyed != (fact.Key != "") {
			return fmt.Errorf("fact %q key does not match fact type %q", fact.ID, fact.Type)
		}
		canonical, err := canonicalValue(fact.Value)
		if err != nil {
			return fmt.Errorf("fact %q value: %w", fact.ID, err)
		}
		if err := validateFactValue(descriptor, canonical); err != nil {
			return fmt.Errorf("fact %q typed value: %w", fact.ID, err)
		}
		baseID := factID(fact.Type, fact.Key, canonical)
		if fact.ID != baseID && !strings.HasPrefix(fact.ID, baseID+":") {
			return fmt.Errorf("fact %q does not match its typed value identity", fact.ID)
		}
		if !validFactUncertainty(fact.Uncertainty.Status) || !validFactAuthority(fact.Authority) {
			return fmt.Errorf("fact %q requires uncertainty and attributed authority", fact.ID)
		}
		if fact.Authority.Kind == "analyzer" && strings.HasPrefix(fact.InputPointer, "/patches/") {
			return fmt.Errorf("analyzer fact %q cannot use patch input_pointer provenance", fact.ID)
		}
		if fact.Evidence == nil {
			return fmt.Errorf("fact %q requires an explicit evidence array", fact.ID)
		}
		if err := validateEvidence(fact.Evidence); err != nil {
			return fmt.Errorf("fact %q evidence: %w", fact.ID, err)
		}
		facts[fact.ID] = fact
	}

	accounted := map[string]bool{}
	for _, accounting := range document.FactAccounting {
		fact, exists := facts[accounting.FactID]
		if !exists {
			return fmt.Errorf("fact accounting references missing fact %q", accounting.FactID)
		}
		if accounted[accounting.FactID] {
			return fmt.Errorf("duplicate accounting for fact %q", accounting.FactID)
		}
		accounted[accounting.FactID] = true
		if accounting.InputPointer != fact.InputPointer {
			return fmt.Errorf("fact %q accounting input_pointer mismatch", accounting.FactID)
		}
		if accounting.Disposition != "rendered" && accounting.Disposition != "retained" {
			return fmt.Errorf("fact %q has invalid accounting disposition %q", accounting.FactID, accounting.Disposition)
		}
		descriptor, _ := descriptorByType(fact.Type)
		want := "retained"
		if descriptor.Rendered && fact.Authority.ClaimClass != "planned" && fact.Authority.ClaimClass != "support" {
			want = "rendered"
		}
		if accounting.Disposition != want {
			return fmt.Errorf("fact %q accounting disposition %q, want %q", accounting.FactID, accounting.Disposition, want)
		}
		if !stringSlicesEqual(accounting.Sections, descriptor.Sections) {
			return fmt.Errorf("fact %q accounting sections do not match registry", accounting.FactID)
		}
	}
	if len(accounted) != len(facts) {
		return fmt.Errorf("fact accounting incomplete: %d facts, %d accounting records", len(facts), len(accounted))
	}
	expectedDispositions := map[string]PatchInputRecord{}
	seenPatchInputs := map[string]bool{}
	for _, input := range document.PatchInputs {
		if strings.TrimSpace(input.PatchID) == "" || seenPatchInputs[input.PatchID] {
			return fmt.Errorf("patch input patch_id must be non-empty and unique: %q", input.PatchID)
		}
		seenPatchInputs[input.PatchID] = true
		if !fingerprintPattern.MatchString(input.ProposalFingerprint) || input.BundleFingerprint != document.AnalyzerInput.BundleFingerprint {
			return fmt.Errorf("patch input %q has invalid proposal or bundle fingerprint", input.PatchID)
		}
		if input.OperationIDs == nil {
			return fmt.Errorf("patch input %q requires an explicit operation_ids array", input.PatchID)
		}
		seenOperations := map[string]bool{}
		for _, operationID := range input.OperationIDs {
			if strings.TrimSpace(operationID) == "" || seenOperations[operationID] {
				return fmt.Errorf("patch input %q operation_id must be non-empty and unique: %q", input.PatchID, operationID)
			}
			seenOperations[operationID] = true
			expectedDispositions[input.PatchID+"\x00"+operationID] = input
		}
	}
	dispositionKeys := map[string]bool{}
	acceptedResults := map[string]ProposalDisposition{}
	for _, disposition := range document.Dispositions {
		key := disposition.PatchID + "\x00" + disposition.OperationID
		if dispositionKeys[key] {
			return fmt.Errorf("duplicate proposal disposition %s/%s", disposition.PatchID, disposition.OperationID)
		}
		dispositionKeys[key] = true
		patchInput, expected := expectedDispositions[key]
		if !expected {
			return fmt.Errorf("proposal disposition %s/%s has no matching patch input", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Status != "accepted" && disposition.Status != "rejected" {
			return fmt.Errorf("proposal %s/%s has invalid disposition %q", disposition.PatchID, disposition.OperationID, disposition.Status)
		}
		if disposition.PatchID == "" || disposition.OperationID == "" || disposition.OriginKind == "" || disposition.OriginID == "" ||
			disposition.ClaimClass == "" || disposition.Action == "" || disposition.FactType == "" || disposition.ProposalReason == "" ||
			disposition.PolicyID == "" || disposition.AuthorizedBy == "" || disposition.DecisionID == "" || disposition.DecidedBy == "" || disposition.DecisionReason == "" {
			return fmt.Errorf("proposal disposition requires proposal, attribution, policy, and decision provenance")
		}
		if !fingerprintPattern.MatchString(disposition.ProposalFingerprint) || disposition.BundleFingerprint != document.AnalyzerInput.BundleFingerprint {
			return fmt.Errorf("proposal %s/%s has invalid proposal or bundle fingerprint", disposition.PatchID, disposition.OperationID)
		}
		if disposition.ProposalFingerprint != patchInput.ProposalFingerprint || disposition.BundleFingerprint != patchInput.BundleFingerprint {
			return fmt.Errorf("proposal %s/%s disposition does not match patch input identity", disposition.PatchID, disposition.OperationID)
		}
		if disposition.OriginKind != "model" && disposition.OriginKind != "human-correction" && disposition.OriginKind != "overlay" {
			return fmt.Errorf("proposal %s/%s has invalid origin kind %q", disposition.PatchID, disposition.OperationID, disposition.OriginKind)
		}
		if !validFactAuthority(FactAuthority{Kind: disposition.OriginKind, Origin: disposition.OriginID, ClaimClass: disposition.ClaimClass}) {
			return fmt.Errorf("proposal %s/%s has invalid attributed claim class", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Action != "add" && disposition.Action != "update" && disposition.Action != "delete" {
			return fmt.Errorf("proposal %s/%s has invalid action %q", disposition.PatchID, disposition.OperationID, disposition.Action)
		}
		if _, ok := descriptorByType(disposition.FactType); !ok {
			return fmt.Errorf("proposal %s/%s has unsupported fact type %q", disposition.PatchID, disposition.OperationID, disposition.FactType)
		}
		if len(disposition.AllowedFactTypes) == 0 {
			return fmt.Errorf("proposal %s/%s disposition requires allowed fact types", disposition.PatchID, disposition.OperationID)
		}
		authorizedType := false
		seenAllowed := map[string]bool{}
		for _, factType := range disposition.AllowedFactTypes {
			if seenAllowed[factType] {
				return fmt.Errorf("proposal %s/%s disposition repeats allowed fact type %q", disposition.PatchID, disposition.OperationID, factType)
			}
			seenAllowed[factType] = true
			if _, ok := descriptorByType(factType); !ok {
				return fmt.Errorf("proposal %s/%s disposition has unsupported allowed fact type %q", disposition.PatchID, disposition.OperationID, factType)
			}
			if factType == disposition.FactType {
				authorizedType = true
			}
		}
		if !authorizedType {
			return fmt.Errorf("proposal %s/%s disposition does not authorize fact type %q", disposition.PatchID, disposition.OperationID, disposition.FactType)
		}
		if disposition.Evidence == nil {
			return fmt.Errorf("proposal %s/%s requires an explicit evidence array", disposition.PatchID, disposition.OperationID)
		}
		if err := validateEvidence(disposition.Evidence); err != nil {
			return fmt.Errorf("proposal %s/%s evidence: %w", disposition.PatchID, disposition.OperationID, err)
		}
		if disposition.Status == "accepted" && len(disposition.Evidence) == 0 {
			return fmt.Errorf("accepted proposal %s/%s requires evidence", disposition.PatchID, disposition.OperationID)
		}
		if disposition.TargetFactID != "" && !factIDPattern.MatchString(disposition.TargetFactID) {
			return fmt.Errorf("proposal %s/%s has invalid target fact ID %q", disposition.PatchID, disposition.OperationID, disposition.TargetFactID)
		}
		if disposition.ResultingFactID != "" && !factIDPattern.MatchString(disposition.ResultingFactID) {
			return fmt.Errorf("proposal %s/%s has invalid resulting fact ID %q", disposition.PatchID, disposition.OperationID, disposition.ResultingFactID)
		}
		if disposition.Status == "accepted" && disposition.ResultingFactID != "" {
			fact, exists := facts[disposition.ResultingFactID]
			if !exists {
				return fmt.Errorf("accepted proposal %s/%s references missing resulting fact %q", disposition.PatchID, disposition.OperationID, disposition.ResultingFactID)
			}
			if fact.Type != disposition.FactType || fact.Authority.Kind != disposition.OriginKind || fact.Authority.Origin != disposition.OriginID || fact.Authority.ClaimClass != disposition.ClaimClass {
				return fmt.Errorf("accepted proposal %s/%s resulting fact attribution mismatch", disposition.PatchID, disposition.OperationID)
			}
			acceptedResults[disposition.ResultingFactID] = disposition
		}
		if disposition.Status == "accepted" && disposition.Action != "delete" && disposition.ResultingFactID == "" {
			return fmt.Errorf("accepted proposal %s/%s requires resulting_fact_id", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Status == "accepted" && disposition.Action == "delete" && disposition.ResultingFactID != "" {
			return fmt.Errorf("accepted delete proposal %s/%s must not have resulting_fact_id", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Status == "rejected" && disposition.ResultingFactID != "" {
			return fmt.Errorf("rejected proposal %s/%s must not have resulting_fact_id", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Action == "add" && disposition.TargetFactID != "" {
			return fmt.Errorf("add proposal %s/%s must not have target_fact_id", disposition.PatchID, disposition.OperationID)
		}
		if disposition.Action != "add" && disposition.TargetFactID == "" {
			return fmt.Errorf("%s proposal %s/%s requires target_fact_id", disposition.Action, disposition.PatchID, disposition.OperationID)
		}
	}
	if len(dispositionKeys) != len(expectedDispositions) {
		for key := range expectedDispositions {
			if !dispositionKeys[key] {
				parts := strings.SplitN(key, "\x00", 2)
				return fmt.Errorf("patch input %s/%s has no proposal disposition", parts[0], parts[1])
			}
		}
	}
	for _, fact := range document.Facts {
		if fact.Authority.Kind == "analyzer" {
			continue
		}
		if !strings.HasPrefix(fact.InputPointer, "/patches/") {
			return fmt.Errorf("non-analyzer fact %q requires patch input_pointer provenance", fact.ID)
		}
		if _, ok := acceptedResults[fact.ID]; !ok {
			return fmt.Errorf("non-analyzer fact %q has no accepted proposal disposition", fact.ID)
		}
	}
	for _, uncertainty := range document.Uncertainty {
		if uncertainty.Scope == "" || uncertainty.Detail == "" {
			return fmt.Errorf("document uncertainty requires scope and detail")
		}
		if err := validateIntegrationStatus(uncertainty.Status); err != nil {
			return err
		}
	}
	if err := validateIntegrationUncertainty(document); err != nil {
		return err
	}
	if document.RenderingView.Component != document.Identity.Component {
		return fmt.Errorf("rendering_view component %q does not match identity component %q", document.RenderingView.Component, document.Identity.Component)
	}
	if document.RenderingView.Metadata.Distribution == "" || document.RenderingView.Metadata.GeneratedBy == "" {
		return fmt.Errorf("rendering_view metadata requires distribution and generated_by")
	}
	if err := validateRenderingView(document); err != nil {
		return err
	}
	return nil
}

func validatePublication(publication Publication, document Document) error {
	if publication.Contract != "structured-component-publication/v1" {
		return fmt.Errorf("unsupported publication contract %q", publication.Contract)
	}
	for label, value := range map[string]string{
		"snapshot_id":                                 publication.SnapshotID,
		"analyzer.content_hash":                       publication.Analyzer.ContentHash,
		"analyzer.bundle_fingerprint":                 publication.Analyzer.BundleFingerprint,
		"analyzer.producer_build_identity":            publication.Analyzer.ProducerBuildIdentity,
		"synthesis.content_hash":                      publication.Synthesis.ContentHash,
		"synthesis.input_bundle_identity":             publication.Synthesis.InputBundleIdentity,
		"synthesis.current_evidence_bundle_identity":  publication.Synthesis.CurrentEvidenceBundleIdentity,
		"synthesis.original_evidence_bundle_identity": publication.Synthesis.OriginalEvidenceBundleIdentity,
	} {
		if !fingerprintPattern.MatchString(value) {
			return fmt.Errorf("publication %s has invalid hash %q", label, value)
		}
	}
	if publication.Analyzer.BundleFingerprint != document.AnalyzerInput.BundleFingerprint ||
		publication.Analyzer.SchemaVersion != document.AnalyzerInput.SchemaVersion ||
		publication.Analyzer.SourceComponent != document.Identity.SourceComponent ||
		publication.Analyzer.Repository != document.Identity.Repository ||
		publication.Analyzer.SourceRevision != document.Identity.SourceRevision ||
		publication.Analyzer.AnalyzerVersion != document.Producers.AnalyzerVersion {
		return fmt.Errorf("publication analyzer binding does not match accepted document inputs")
	}
	if publication.Markdown.Path != document.Identity.Component+".md" || publication.Markdown.RendererVersion != document.Producers.RendererVersion {
		return fmt.Errorf("publication Markdown binding does not match accepted document identity")
	}
	switch publication.Synthesis.State {
	case "synthesized":
		if !publication.Synthesis.ProducingModelEligible || strings.TrimSpace(publication.Synthesis.AcceptedResponseIdentity) == "" {
			return fmt.Errorf("synthesized publication requires an eligible producing model and accepted response")
		}
	case "deterministic-only", "historical-response-missing":
		if publication.Synthesis.ProducingModelEligible || publication.Synthesis.AcceptedResponseIdentity != "" {
			return fmt.Errorf("%s publication cannot claim an eligible producing model response", publication.Synthesis.State)
		}
	default:
		return fmt.Errorf("publication synthesis state %q is invalid", publication.Synthesis.State)
	}
	if publication.Diagram.State != "unavailable" && publication.Diagram.State != "available" && publication.Diagram.State != "failed" {
		return fmt.Errorf("publication diagram state %q is invalid", publication.Diagram.State)
	}
	if len(publication.AcceptedInputs.RunRecord) == 0 || len(publication.AcceptedInputs.CurrentEvidenceBundle) == 0 || len(publication.AcceptedInputs.OriginalEvidenceBundle) == 0 {
		return fmt.Errorf("publication requires durable run record and evidence bundles")
	}
	return nil
}

func validateRenderingView(document Document) error {
	input := model.Input{
		Component: document.Identity.SourceComponent, Repo: document.Identity.Repository,
		CommitSHA: document.Identity.SourceRevision, ExtractedAt: document.AnalyzerInput.ExtractedAt,
		AnalyzerVersion: document.Producers.AnalyzerVersion, SchemaVersion: document.AnalyzerInput.SchemaVersion,
	}
	composed, err := compose(input, implementationFacts(document.Facts))
	if err != nil {
		return fmt.Errorf("reconstruct rendering_view facts: %w", err)
	}
	var componentMap *model.ComponentMap
	if document.AssemblyInputs.ComponentMap != nil {
		componentMap = &document.AssemblyInputs.ComponentMap.Value
	}
	expected := normalize.Input(composed, normalize.Options{
		Distribution: document.RenderingConfig.Distribution,
		GeneratedBy:  document.RenderingConfig.GeneratedBy,
		ComponentMap: componentMap,
	})
	if !reflect.DeepEqual(expected, document.RenderingView) {
		return fmt.Errorf("rendering_view does not match accepted implementation facts, parent mapping inputs, and rendering configuration")
	}
	return nil
}

func validateAssemblyInputs(inputs AssemblyInputs) error {
	if inputs.ComponentMap == nil {
		return nil
	}
	componentMap := inputs.ComponentMap
	if componentMap.Origin.Kind != "orchestrator" || strings.TrimSpace(componentMap.Origin.ID) == "" {
		return fmt.Errorf("component-map assembly input requires attributed orchestrator origin")
	}
	if !fingerprintPattern.MatchString(componentMap.ContentFingerprint) {
		return fmt.Errorf("component-map assembly input has invalid content_fingerprint %q", componentMap.ContentFingerprint)
	}
	raw, err := json.Marshal(componentMap.Value)
	if err != nil {
		return fmt.Errorf("encode component-map assembly input: %w", err)
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return fmt.Errorf("canonicalize component-map assembly input: %w", err)
	}
	if digest(canonical) != componentMap.ContentFingerprint {
		return fmt.Errorf("component-map assembly input content_fingerprint mismatch")
	}
	if componentMap.Value.Components == nil {
		return fmt.Errorf("component-map assembly input requires explicit components")
	}
	return nil
}

func ValidateSections(sections []model.StructuredSection) error {
	seen := map[string]bool{}
	lastOrder := -1
	for _, section := range sections {
		definition, known := sectionDefinition(section.ID)
		if !known {
			return fmt.Errorf("unsupported structured section %q", section.ID)
		}
		if seen[section.ID] {
			return fmt.Errorf("duplicate structured section %q", section.ID)
		}
		seen[section.ID] = true
		if definition.Order < lastOrder {
			return fmt.Errorf("structured sections are not in registry order at %q", section.ID)
		}
		lastOrder = definition.Order
		if section.Status != "documented" && section.Status != "unresolved" && section.Status != "not-applicable" {
			return fmt.Errorf("structured section %q has invalid status %q", section.ID, section.Status)
		}
		if !validSectionAuthority(section.Authority) {
			return fmt.Errorf("structured section %q requires attributed authority and claim class", section.ID)
		}
		if section.Blocks == nil {
			return fmt.Errorf("structured section %q requires an explicit blocks array", section.ID)
		}
		if section.Status == "unresolved" && strings.TrimSpace(section.Uncertainty) == "" {
			return fmt.Errorf("unresolved structured section %q requires uncertainty", section.ID)
		}
		if section.Uncertainty != "" {
			if err := validateInlineContent(section.Uncertainty); err != nil {
				return fmt.Errorf("structured section %q uncertainty: %w", section.ID, err)
			}
		}
		if section.Evidence == nil {
			return fmt.Errorf("structured section %q requires an explicit evidence array", section.ID)
		}
		seenTables := map[string]bool{}
		lastTableOrder := -1
		for _, block := range section.Blocks {
			switch block.Type {
			case "paragraph":
				if strings.TrimSpace(block.Text) == "" || len(block.Items) != 0 || block.TableID != "" || len(block.Rows) != 0 {
					return fmt.Errorf("section %q paragraph requires text only", section.ID)
				}
				if err := validateInlineContent(block.Text); err != nil {
					return fmt.Errorf("section %q paragraph: %w", section.ID, err)
				}
			case "list":
				if block.Text != "" || len(block.Items) == 0 || block.TableID != "" || len(block.Rows) != 0 {
					return fmt.Errorf("section %q list requires items only", section.ID)
				}
				for _, item := range block.Items {
					if err := validateInlineContent(item); err != nil {
						return fmt.Errorf("section %q list item: %w", section.ID, err)
					}
				}
			case "table":
				if block.Text != "" || len(block.Items) != 0 || block.TableID == "" || block.Rows == nil {
					return fmt.Errorf("section %q table requires table_id and explicit rows only", section.ID)
				}
				definition, ok := tableDefinition(block.TableID)
				if !ok || definition.SectionID != section.ID {
					return fmt.Errorf("section %q has unsupported table %q", section.ID, block.TableID)
				}
				if len(block.Rows) == 0 {
					return fmt.Errorf("section %q table %q requires at least one row", section.ID, block.TableID)
				}
				if seenTables[block.TableID] {
					return fmt.Errorf("section %q repeats table %q", section.ID, block.TableID)
				}
				seenTables[block.TableID] = true
				if definition.Order < lastTableOrder {
					return fmt.Errorf("section %q tables are not in registry order at %q", section.ID, block.TableID)
				}
				lastTableOrder = definition.Order
				for rowIndex, row := range block.Rows {
					if len(row.Cells) != len(definition.Headers) {
						return fmt.Errorf("section %q table %q row %d has %d cells, want %d", section.ID, block.TableID, rowIndex, len(row.Cells), len(definition.Headers))
					}
					for _, cell := range row.Cells {
						if err := validateInlineContent(cell); err != nil {
							return fmt.Errorf("section %q table %q row %d: %w", section.ID, block.TableID, rowIndex, err)
						}
					}
					if len(row.Evidence) == 0 {
						return fmt.Errorf("section %q table %q row %d requires evidence", section.ID, block.TableID, rowIndex)
					}
					if err := validateSectionEvidence(row.Evidence); err != nil {
						return fmt.Errorf("section %q table %q row %d evidence: %w", section.ID, block.TableID, rowIndex, err)
					}
				}
			default:
				return fmt.Errorf("section %q has unsupported block type %q", section.ID, block.Type)
			}
		}
		if err := validateSectionEvidence(section.Evidence); err != nil {
			return fmt.Errorf("section %q evidence: %w", section.ID, err)
		}
		if len(section.Evidence) == 0 {
			return fmt.Errorf("structured section %q status %q requires evidence", section.ID, section.Status)
		}
	}
	return nil
}

func validateInlineContent(value string) error {
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return fmt.Errorf("content must be a single line without control characters")
		}
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("content must be a single inline line")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("content must not contain NUL")
	}
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "    ") {
		return fmt.Errorf("indented code blocks are not allowed")
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("content must not be empty")
	}
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
		return fmt.Errorf("fenced code blocks are not allowed")
	}
	if headingPattern.MatchString(trimmed) {
		return fmt.Errorf("headings are not allowed")
	}
	if thematicBreakPattern.MatchString(trimmed) {
		return fmt.Errorf("thematic breaks are not allowed")
	}
	if strings.HasPrefix(trimmed, ">") {
		return fmt.Errorf("block quotes are not allowed")
	}
	if (len(trimmed) >= 2 && strings.ContainsRune("-+*", rune(trimmed[0])) && (trimmed[1] == ' ' || trimmed[1] == '\t')) || orderedListPattern.MatchString(trimmed) {
		return fmt.Errorf("nested list syntax is not allowed")
	}
	if rawHTMLPattern.MatchString(trimmed) {
		return fmt.Errorf("raw HTML is not allowed")
	}
	if strings.Contains(trimmed, "|") {
		return fmt.Errorf("raw table syntax is not allowed")
	}
	if linkPattern.MatchString(trimmed) {
		return fmt.Errorf("raw Markdown links are not allowed")
	}
	return nil
}

func validateReuseRecord(record ReuseRecord, document Document) error {
	if record.SchemaVersion != "1.0.0" {
		return fmt.Errorf("unsupported reuse record schema_version %q", record.SchemaVersion)
	}
	for name, value := range map[string]string{
		"prior_snapshot_id": record.PriorSnapshotID,
		"prior_platform":    record.PriorPlatform,
		"target_platform":   record.TargetPlatform,
		"response_identity": record.ResponseIdentity,
		"reason":            record.Reason,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("reuse record requires normalized %s", name)
		}
	}
	if record.TargetPlatform != document.Identity.VersionScope {
		return fmt.Errorf("reuse target_platform %q does not match document version_scope %q", record.TargetPlatform, document.Identity.VersionScope)
	}
	for name, value := range map[string]string{
		"prior_document_hash":     record.PriorDocumentHash,
		"original_synthesis_hash": record.OriginalSynthesisHash,
		"target_exact_input":      record.TargetExactInput,
		"target_semantic_input":   record.TargetSemanticInput,
		"comparison_hash":         record.ComparisonHash,
	} {
		if !fingerprintPattern.MatchString(value) {
			return fmt.Errorf("reuse record has invalid %s %q", name, value)
		}
	}
	if len(record.RevalidationChecks) == 0 {
		return fmt.Errorf("reuse record requires explicit successful revalidation_checks")
	}
	seen := map[string]bool{}
	for _, check := range record.RevalidationChecks {
		if strings.TrimSpace(check) == "" || seen[check] {
			return fmt.Errorf("reuse record revalidation_checks must be non-empty and unique")
		}
		seen[check] = true
	}
	return nil
}

func validateSectionEvidence(evidence []model.StructuredEvidenceRef) error {
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
			return fmt.Errorf("invalid evidence range %d-%d", reference.StartLine, reference.EndLine)
		}
		if strings.TrimSpace(reference.Revision) == "" {
			return fmt.Errorf("missing evidence revision for %q", reference.Path)
		}
		if err := validateEvidenceAtom(reference.Revision); err != nil {
			return fmt.Errorf("invalid evidence revision for %q: %w", reference.Path, err)
		}
	}
	return nil
}

// Evidence atoms are rendered as citations and must remain on one physical
// Markdown line. Reject ASCII controls plus Unicode line/paragraph separators;
// ordinary repository path and revision punctuation remains valid.
func validateEvidenceAtom(value string) error {
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return fmt.Errorf("must be a single line without control characters")
		}
	}
	return nil
}

func validateIntegrationUncertainty(document Document) error {
	count := 0
	for _, uncertainty := range document.Uncertainty {
		if uncertainty.Scope != "component-integration" {
			continue
		}
		count++
		if uncertainty.Status != document.Identity.IntegrationStatus {
			return fmt.Errorf("component-integration uncertainty status %q does not match identity integration_status %q", uncertainty.Status, document.Identity.IntegrationStatus)
		}
	}
	if count != 1 {
		return fmt.Errorf("structured document requires exactly one component-integration uncertainty record, got %d", count)
	}
	return nil
}

func validSectionAuthority(authority model.StructuredAuthority) bool {
	return validFactAuthority(FactAuthority{Kind: authority.Kind, Origin: authority.Origin, ClaimClass: authority.ClaimClass}) && authority.Kind != "analyzer"
}

func validateIntegrationStatus(status string) error {
	switch status {
	case "current", "planned", "not-integrated", "unknown":
		return nil
	default:
		return fmt.Errorf("invalid integration status %q", status)
	}
}

func validFactUncertainty(status string) bool {
	switch status {
	case "extracted", "unknown", "not-extracted", "unresolved", "planned", "not-verified":
		return true
	default:
		return false
	}
}

func validFactAuthority(authority FactAuthority) bool {
	if authority.Origin == "" {
		return false
	}
	if authority.Kind == "analyzer" {
		return authority.Origin == "analyzer-input" && authority.ClaimClass == "implementation"
	}
	switch authority.Kind {
	case "model", "human-correction", "overlay":
	default:
		return false
	}
	switch authority.ClaimClass {
	case "implementation", "support", "planned", "correction":
		return true
	default:
		return false
	}
}

func stringSlicesEqual(left, right []string) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return bytes.Equal(leftJSON, rightJSON)
}
