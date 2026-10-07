// Package structured implements the phase-one accepted component document.
// It deliberately has no publication, model invocation, or reuse behavior.
package structured

import (
	"encoding/json"

	"github.com/jctanner/arch-analyzer/internal/model"
)

const (
	DocumentSchemaVersion          = "1.0.0"
	PublishedDocumentSchemaVersion = "1.1.0"
	PatchSchemaVersion             = "1.0.0"
	PolicySchemaVersion            = "1.0.0"
	NormalizerVersion              = "structured-component-normalizer/v1"
	RendererVersion                = "arch-analyzer-markdown/v1"
)

type Document struct {
	SchemaVersion   string                    `json:"schema_version"`
	Identity        Identity                  `json:"identity"`
	Producers       Producers                 `json:"producers"`
	AnalyzerInput   AnalyzerInput             `json:"analyzer_input"`
	RenderingConfig RenderingConfig           `json:"rendering_config"`
	AssemblyInputs  *AssemblyInputs           `json:"assembly_inputs"`
	Facts           []Fact                    `json:"facts"`
	FactAccounting  []FactAccounting          `json:"fact_accounting"`
	Sections        []model.StructuredSection `json:"sections"`
	PatchInputs     []PatchInputRecord        `json:"patch_inputs"`
	Dispositions    []ProposalDisposition     `json:"proposal_dispositions"`
	Uncertainty     []Uncertainty             `json:"uncertainty"`
	Reuse           *ReuseRecord              `json:"reuse,omitempty"`
	Publication     *Publication              `json:"publication,omitempty"`
	RenderingView   model.Document            `json:"rendering_view"`
}

// Publication is trusted parent-produced binding and recovery provenance.  It
// intentionally does not contain a document or Markdown hash: the exact
// document bytes are the authority identity and the Markdown derivative names
// those bytes, avoiding a circular hash relationship.
type Publication struct {
	Contract       string                  `json:"contract"`
	SnapshotID     string                  `json:"snapshot_id"`
	Analyzer       PublishedAnalyzer       `json:"analyzer"`
	Synthesis      PublishedSynthesis      `json:"synthesis"`
	AcceptedInputs PublishedAcceptedInputs `json:"accepted_inputs"`
	Markdown       PublishedMarkdown       `json:"markdown"`
	Diagram        PublishedDiagram        `json:"diagram"`
}

type PublishedAnalyzer struct {
	ContentHash           string `json:"content_hash"`
	BundleFingerprint     string `json:"bundle_fingerprint"`
	SchemaVersion         string `json:"schema_version"`
	SourceComponent       string `json:"source_component"`
	Repository            string `json:"repository"`
	SourceRevision        string `json:"source_revision"`
	AnalyzerVersion       string `json:"analyzer_version"`
	ProducerBuildIdentity string `json:"producer_build_identity"`
}

type PublishedSynthesis struct {
	ContentHash                    string `json:"content_hash"`
	State                          string `json:"state"`
	InputBundleIdentity            string `json:"input_bundle_identity"`
	CurrentEvidenceBundleIdentity  string `json:"current_evidence_bundle_identity"`
	OriginalEvidenceBundleIdentity string `json:"original_evidence_bundle_identity"`
	AcceptedResponseIdentity       string `json:"accepted_response_identity,omitempty"`
	ProducingModelEligible         bool   `json:"producing_model_eligible"`
}

// PublishedAcceptedInputs retains the complete private acceptance record and
// both evidence bundles needed to audit and reuse a snapshot after disposable
// logs and private staging have been removed.
type PublishedAcceptedInputs struct {
	RunRecord              json.RawMessage `json:"run_record"`
	CurrentEvidenceBundle  json.RawMessage `json:"current_evidence_bundle"`
	OriginalEvidenceBundle json.RawMessage `json:"original_evidence_bundle"`
}

type PublishedMarkdown struct {
	Path            string `json:"path"`
	RendererVersion string `json:"renderer_version"`
}

type PublishedDiagram struct {
	State             string `json:"state"`
	GeneratorIdentity string `json:"generator_identity,omitempty"`
}

// ReuseRecord is trusted parent-produced provenance for a verified
// whole-component reuse hit. It is deliberately absent from normal
// deterministic and newly synthesized documents.
type ReuseRecord struct {
	SchemaVersion         string   `json:"schema_version"`
	PriorSnapshotID       string   `json:"prior_snapshot_id"`
	PriorPlatform         string   `json:"prior_platform"`
	TargetPlatform        string   `json:"target_platform"`
	PriorDocumentHash     string   `json:"prior_document_hash"`
	OriginalSynthesisHash string   `json:"original_synthesis_hash"`
	ResponseIdentity      string   `json:"response_identity"`
	TargetExactInput      string   `json:"target_exact_input"`
	TargetSemanticInput   string   `json:"target_semantic_input"`
	ComparisonHash        string   `json:"comparison_hash"`
	Reason                string   `json:"reason"`
	RevalidationChecks    []string `json:"revalidation_checks"`
}

type Identity struct {
	Component         string   `json:"component"`
	SourceComponent   string   `json:"source_component"`
	Repository        string   `json:"repository"`
	SourceRevision    string   `json:"source_revision"`
	VersionScope      string   `json:"version_scope"`
	IntegrationStatus string   `json:"integration_status"`
	Aliases           []string `json:"aliases"`
}

type Producers struct {
	AnalyzerVersion   string `json:"analyzer_version"`
	NormalizerVersion string `json:"normalizer_version"`
	RendererVersion   string `json:"renderer_version"`
}

type AnalyzerInput struct {
	SchemaVersion     string `json:"schema_version"`
	BundleFingerprint string `json:"bundle_fingerprint"`
	ExtractedAt       string `json:"extracted_at"`
}

type RenderingConfig struct {
	Distribution string `json:"distribution"`
	GeneratedBy  string `json:"generated_by"`
}

// AssemblyInputs are trusted parent-supplied mapping inputs. They are not
// analyzer facts and therefore do not participate in analyzer fact accounting.
// The complete components/provenance projection consumed by normalization is
// retained so derived rendering data can be recomputed during validation.
type AssemblyInputs struct {
	ComponentMap *ComponentMapInput `json:"component_map"`
}

type ComponentMapInput struct {
	ContentFingerprint string              `json:"content_fingerprint"`
	Origin             AssemblyInputOrigin `json:"origin"`
	Value              model.ComponentMap  `json:"value"`
}

type AssemblyInputOrigin struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Fact is a typed, immutable analyzer fact. Value is decoded against the Go
// analyzer type selected by Type before acceptance; it is RawMessage here so a
// single envelope can retain every analyzer fact family without flattening it
// into renderer display columns.
type Fact struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Key          string          `json:"key,omitempty"`
	Ordinal      int             `json:"ordinal"`
	Value        json.RawMessage `json:"value"`
	InputPointer string          `json:"input_pointer"`
	Evidence     []EvidenceRef   `json:"evidence"`
	Uncertainty  FactUncertainty `json:"uncertainty"`
	Authority    FactAuthority   `json:"authority"`
}

type EvidenceRef struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Revision  string `json:"revision"`
}

type FactUncertainty struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type FactAuthority struct {
	Kind       string `json:"kind"`
	Origin     string `json:"origin"`
	ClaimClass string `json:"claim_class"`
}

type FactAccounting struct {
	FactID       string   `json:"fact_id"`
	InputPointer string   `json:"input_pointer"`
	Disposition  string   `json:"disposition"`
	Sections     []string `json:"sections"`
}

type Uncertainty struct {
	Scope  string `json:"scope"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type PatchSet struct {
	SchemaVersion     string           `json:"schema_version"`
	PatchID           string           `json:"patch_id"`
	BundleFingerprint string           `json:"bundle_fingerprint"`
	Operations        []PatchOperation `json:"operations"`
}

type PatchOrigin struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	ClaimClass string `json:"claim_class"`
}

type PatchAuthority struct {
	Actor            string   `json:"actor"`
	AllowedFactTypes []string `json:"allowed_fact_types"`
}

type PatchOperation struct {
	OperationID  string          `json:"operation_id"`
	Action       string          `json:"action"`
	FactType     string          `json:"fact_type"`
	TargetFactID string          `json:"target_fact_id,omitempty"`
	Key          string          `json:"key,omitempty"`
	Value        json.RawMessage `json:"value,omitempty"`
	Evidence     []EvidenceRef   `json:"evidence"`
	Reason       string          `json:"reason"`
}

type PatchInputRecord struct {
	PatchID             string   `json:"patch_id"`
	ProposalFingerprint string   `json:"proposal_fingerprint"`
	BundleFingerprint   string   `json:"bundle_fingerprint"`
	OperationIDs        []string `json:"operation_ids"`
}

// AssemblyPolicy is supplied through a separate trusted orchestrator/CLI
// channel. Nothing in a proposal can grant authority or decide its operations.
type AssemblyPolicy struct {
	SchemaVersion       string             `json:"schema_version"`
	PolicyID            string             `json:"policy_id"`
	PatchID             string             `json:"patch_id"`
	ProposalFingerprint string             `json:"proposal_fingerprint"`
	BundleFingerprint   string             `json:"bundle_fingerprint"`
	VersionScope        string             `json:"version_scope"`
	Origin              PatchOrigin        `json:"origin"`
	Authority           PatchAuthority     `json:"authority"`
	Decisions           []AssemblyDecision `json:"decisions"`
}

type AssemblyDecision struct {
	DecisionID  string `json:"decision_id"`
	OperationID string `json:"operation_id"`
	Decision    string `json:"decision"`
	DecidedBy   string `json:"decided_by"`
	Reason      string `json:"reason"`
}

type ProposalDisposition struct {
	PatchID             string        `json:"patch_id"`
	ProposalFingerprint string        `json:"proposal_fingerprint"`
	BundleFingerprint   string        `json:"bundle_fingerprint"`
	OperationID         string        `json:"operation_id"`
	Action              string        `json:"action"`
	FactType            string        `json:"fact_type"`
	OriginKind          string        `json:"origin_kind"`
	OriginID            string        `json:"origin_id"`
	ClaimClass          string        `json:"claim_class"`
	Status              string        `json:"status"`
	TargetFactID        string        `json:"target_fact_id,omitempty"`
	ResultingFactID     string        `json:"resulting_fact_id,omitempty"`
	Evidence            []EvidenceRef `json:"evidence"`
	ProposalReason      string        `json:"proposal_reason"`
	PolicyID            string        `json:"policy_id"`
	AuthorizedBy        string        `json:"authorized_by"`
	AllowedFactTypes    []string      `json:"allowed_fact_types"`
	DecisionID          string        `json:"decision_id"`
	DecidedBy           string        `json:"decided_by"`
	DecisionReason      string        `json:"decision_reason"`
}

type Options struct {
	Distribution      string
	GeneratedBy       string
	VersionScope      string
	IntegrationStatus string
	Aliases           []string
	ComponentMap      *model.ComponentMap
	ComponentMapID    string
	Sections          []model.StructuredSection
	Patches           []PatchSet
	AssemblyPolicies  []AssemblyPolicy
	Reuse             *ReuseRecord
}
