package documentdata_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	analyzerdocument "github.com/jctanner/arch-analyzer/pkg/document"
	"github.com/jctanner/arch-query/internal/documentdata"
	"github.com/jctanner/arch-query/internal/markdown"
	"github.com/jctanner/arch-query/internal/types"
)

func TestEmbeddedSchemaMatchesAcceptedContract(t *testing.T) {
	want, err := os.ReadFile("../../../../schemas/structured-component-document-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("structured-component-document-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("embedded accepted-document schema differs from the phase-one contract")
	}
}

func TestEmbeddedSynthesisSchemaMatchesAcceptedContract(t *testing.T) {
	want, err := os.ReadFile("../../../../schemas/structured-component-synthesis-envelope-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("structured-component-synthesis-envelope-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("embedded synthesis schema differs from the central contract")
	}
}

func TestAcceptedFixturesPassSharedValidation(t *testing.T) {
	for _, path := range []string{
		"testdata/rejected-document.json",
		"testdata/rhoai-test/policy/document.json",
		"testdata/rhoai.next/praxis-policy/document.json",
		"testdata/rhoai.next/typed-only/document.json",
	} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := analyzerdocument.ValidateJSON(raw); err != nil {
				t.Fatalf("fixture does not satisfy the shared accepted-document validator: %v", err)
			}
		})
	}
}

func TestParseAcceptedDocumentAllowsValidatedReuseMetadata(t *testing.T) {
	base := readJSONFixture(t, "testdata/rhoai.next/praxis-policy/document.json")
	hash := "sha256:" + strings.Repeat("a", 64)
	base["reuse"] = map[string]any{
		"schema_version":          "1.0.0",
		"prior_snapshot_id":       "snapshot-1",
		"prior_platform":          "rhoai-old",
		"target_platform":         "rhoai.next",
		"prior_document_hash":     hash,
		"original_synthesis_hash": hash,
		"response_identity":       "response-1",
		"target_exact_input":      hash,
		"target_semantic_input":   hash,
		"comparison_hash":         hash,
		"reason":                  "verified semantic and dependency match",
		"revalidation_checks":     []any{"current_acceptance_rules", "document_schema"},
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"document.json": {Data: raw}}
	document, err := documentdata.Parse(fsys, "document.json", "praxis-policy", "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	if document.Name != "praxis-policy" {
		t.Fatalf("accepted projection changed with reuse metadata: %#v", document)
	}

	base["reuse"].(map[string]any)["target_platform"] = "rhoai-other"
	invalid, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	_, err = documentdata.Parse(
		fstest.MapFS{"document.json": {Data: invalid}},
		"document.json", "praxis-policy", "rhoai.next",
	)
	if err == nil || !strings.Contains(err.Error(), "target_platform") {
		t.Fatalf("invalid reuse target error=%v", err)
	}
}

func TestParseAcceptedDocumentMatchesRenderedMarkdownTypedProjection(t *testing.T) {
	fsys := os.DirFS("testdata")
	accepted, err := documentdata.Parse(fsys, "rhoai.next/praxis-policy/document.json", "praxis-policy", "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := markdown.ParseComponentDoc(fsys, "rhoai.next/praxis-policy.md")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := legacyProjection(accepted), legacyProjection(legacy); !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("accepted projection differs from equivalent Markdown\ngot: %s\nwant: %s", gotJSON, wantJSON)
	}
	if accepted.SourceComponent != "policy" || accepted.IntegrationStatus != "not-integrated" || !reflect.DeepEqual(accepted.Aliases, []string{"policy"}) {
		t.Fatalf("prefixed non-integrated identity lost: %#v", accepted)
	}
	if len(accepted.RBACRoles) != 4 {
		t.Fatalf("RBAC rules = %d, want 4", len(accepted.RBACRoles))
	}
	wantRules := []types.RBACRole{
		{RoleName: "policy-reader", APIGroup: "example.io", Resources: "policies", Verbs: "get, list"},
		{RoleName: "policy-reader", NonResourceURLs: "/metrics", Verbs: "get"},
		{RoleName: "policy-reader", APIGroup: "example.io", Resources: "policies/status", NonResourceURLs: "/healthz", Verbs: "get"},
		{RoleName: "policy-reader", Verbs: "get"},
	}
	if !reflect.DeepEqual(accepted.RBACRoles, wantRules) {
		t.Fatalf("RBAC variants were not preserved: %#v", accepted.RBACRoles)
	}
	if len(accepted.SourceCitations) == 0 || accepted.SourceCitations[0].Revision != "0123456789abcdef" {
		t.Fatalf("revision-bound citations missing: %#v", accepted.SourceCitations)
	}
}

func TestPurposeProjectionTracksCurrentProducerRenderer(t *testing.T) {
	raw, err := os.ReadFile("testdata/rhoai.next/praxis-policy/document.json")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := analyzerdocument.RenderBodyJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"document.json": {Data: raw},
		"rendered.md":   {Data: rendered},
	}
	accepted, err := documentdata.Parse(fsys, "document.json", "praxis-policy", "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	current, err := markdown.ParseComponentDoc(fsys, "rendered.md")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Purpose != current.Purpose || accepted.PurposeFull != current.PurposeFull {
		t.Fatalf("query purpose projection drifted from current producer renderer\naccepted: %q / %q\nrenderer: %q / %q", accepted.Purpose, accepted.PurposeFull, current.Purpose, current.PurposeFull)
	}
}

func TestParseAcceptedDocumentMapsTypedOnlyFacts(t *testing.T) {
	doc, err := documentdata.Parse(os.DirFS("testdata"), "rhoai.next/typed-only/document.json", "typed-only", "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.ControllerWatches) != 1 || doc.ControllerWatches[0].Source != "controllers/typed.go:12" {
		t.Fatalf("controller watches = %#v", doc.ControllerWatches)
	}
	if len(doc.Webhooks) != 1 || doc.Webhooks[0].ServiceRef != "typed-webhook" || doc.Webhooks[0].Port != 9443 || len(doc.Webhooks[0].Rules) != 1 || len(doc.Webhooks[0].Sources) != 1 {
		t.Fatalf("webhooks = %#v", doc.Webhooks)
	}
	if len(doc.ExternalWebhooks) != 1 || doc.ExternalWebhooks[0].Component != "external-policy" {
		t.Fatalf("external webhooks = %#v", doc.ExternalWebhooks)
	}
	if len(doc.Dockerfiles) != 1 || doc.Dockerfiles[0].BaseImage != "registry.example/base:1" {
		t.Fatalf("dockerfiles = %#v", doc.Dockerfiles)
	}
	if got := doc.CrossCuttingEvidence["fips"]; len(got) != 1 || got[0].Claim != "uses system crypto" {
		t.Fatalf("cross-cutting evidence = %#v", doc.CrossCuttingEvidence)
	}
	for _, factType := range []string{"controller_watch", "webhook", "dockerfile", "cross_cutting_evidence"} {
		found := false
		for _, citation := range doc.SourceCitations {
			if citation.FactType == factType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no citation record for typed fact %s", factType)
		}
	}
	gapFound := false
	for _, gap := range doc.ProvenanceGaps {
		if gap.FactType == "external_webhook" {
			gapFound = true
		}
	}
	if !gapFound {
		t.Fatalf("evidence-free fact did not produce an explicit provenance gap: %#v", doc.ProvenanceGaps)
	}
}

func TestParseAcceptedDocumentMapsProposalDisposition(t *testing.T) {
	doc, err := documentdata.Parse(os.DirFS("testdata"), "rhoai-test/policy/document.json", "policy", "rhoai-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.ProposalDispositions) != 1 {
		t.Fatalf("proposal dispositions = %#v", doc.ProposalDispositions)
	}
	disposition := doc.ProposalDispositions[0]
	if disposition.Status != "accepted" || disposition.ResultingFactID == "" || disposition.DecidedBy != "reviewer@example" || len(disposition.Evidence) != 1 {
		t.Fatalf("proposal disposition lost provenance: %#v", disposition)
	}
}

func TestRejectedProposalIsDispositionOnly(t *testing.T) {
	doc, err := documentdata.Parse(os.DirFS("testdata"), "rejected-document.json", "policy", "rhoai-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.ProposalDispositions) != 1 || doc.ProposalDispositions[0].Status != "rejected" || doc.ProposalDispositions[0].ResultingFactID != "" {
		t.Fatalf("rejected proposal disposition = %#v", doc.ProposalDispositions)
	}
	if strings.Contains(doc.Purpose, "corrected summary") || strings.Contains(doc.PurposeFull, "corrected summary") {
		t.Fatalf("rejected proposal was treated as an accepted fact: %q / %q", doc.Purpose, doc.PurposeFull)
	}
}

func TestParseAcceptedDocumentRejectsInvalidContractAndIntegrity(t *testing.T) {
	raw, err := os.ReadFile("testdata/rhoai.next/praxis-policy/document.json")
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err := json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name              string
		mutate            func(map[string]any)
		expectedComponent string
		want              string
	}{
		{"unsupported version", func(value map[string]any) { value["schema_version"] = "2.0.0" }, "", "schema_version"},
		{"wrong type", func(value map[string]any) { value["facts"] = "not-an-array" }, "", "cannot unmarshal"},
		{"missing required", func(value map[string]any) { delete(value, "rendering_view") }, "", "rendering_view component"},
		{"publication identity mismatch", func(map[string]any) {}, "policy", "publication directory"},
		{"render view component mismatch", func(value map[string]any) { value["rendering_view"].(map[string]any)["component"] = "policy" }, "", "rendering_view component"},
		{"version mismatch", func(value map[string]any) { value["identity"].(map[string]any)["version_scope"] = "rhoai.other" }, "", "resolved version"},
		{"fact identity tamper", func(value map[string]any) { value["facts"].([]any)[0].(map[string]any)["value"] = "tampered" }, "", "typed value identity"},
		{"accounting removed", func(value map[string]any) { value["fact_accounting"] = value["fact_accounting"].([]any)[1:] }, "", "accounting incomplete"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyRaw, _ := json.Marshal(base)
			var value map[string]any
			_ = json.Unmarshal(copyRaw, &value)
			test.mutate(value)
			mutated, _ := json.Marshal(value)
			fsys := fstest.MapFS{"document.json": {Data: mutated}}
			expectedComponent := test.expectedComponent
			if expectedComponent == "" {
				expectedComponent = "praxis-policy"
			}
			_, parseErr := documentdata.Parse(fsys, "document.json", expectedComponent, "rhoai.next")
			if parseErr == nil || !strings.Contains(parseErr.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", parseErr, test.want)
			}
		})
	}
}

func TestParseAcceptedDocumentRejectsSharedSemanticTampering(t *testing.T) {
	base := readJSONFixture(t, "testdata/rhoai.next/praxis-policy/document.json")
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "dependency role",
			mutate: func(value map[string]any) {
				view := value["rendering_view"].(map[string]any)
				view["internal_dependencies"] = []any{map[string]any{
					"component": "tampered", "interaction_type": "HTTP", "role": "caller", "purpose": "not backed by facts",
				}}
			},
			want: "rendering_view does not match",
		},
		{
			name: "RBAC",
			mutate: func(value map[string]any) {
				roles := value["rendering_view"].(map[string]any)["cluster_roles"].([]any)
				roles[0].(map[string]any)["verbs"] = "delete"
			},
			want: "rendering_view does not match",
		},
		{
			name: "purpose",
			mutate: func(value map[string]any) {
				value["rendering_view"].(map[string]any)["purpose"] = "Contradicts unchanged facts."
			},
			want: "rendering_view does not match",
		},
		{
			name: "lineage",
			mutate: func(value map[string]any) {
				value["rendering_view"].(map[string]any)["repo_lineage"] = []any{}
			},
			want: "rendering_view does not match",
		},
		{
			name: "accounting disposition",
			mutate: func(value map[string]any) {
				value["fact_accounting"].([]any)[0].(map[string]any)["disposition"] = "retained"
			},
			want: "accounting disposition",
		},
		{
			name: "accounting reference",
			mutate: func(value map[string]any) {
				value["fact_accounting"].([]any)[0].(map[string]any)["fact_id"] = "fact:summary:0000000000000000000000000000000000000000000000000000000000000000"
			},
			want: "references missing fact",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneJSON(t, base)
			test.mutate(value)
			mutated, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			fsys := fstest.MapFS{"document.json": {Data: mutated}}
			doc, parseErr := documentdata.Parse(fsys, "document.json", "praxis-policy", "rhoai.next")
			if doc != nil || parseErr == nil || !strings.Contains(parseErr.Error(), "shared analyzer validator") || !strings.Contains(parseErr.Error(), test.want) {
				t.Fatalf("Parse() = %#v, %v; want shared validation error containing %q", doc, parseErr, test.want)
			}
		})
	}
}

func TestParseAcceptedDocumentRejectsProposalTampering(t *testing.T) {
	base := readJSONFixture(t, "testdata/rhoai-test/policy/document.json")
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "proposal identity",
			mutate: func(value map[string]any) {
				value["proposal_dispositions"].([]any)[0].(map[string]any)["proposal_fingerprint"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
			},
			want: "does not match patch input identity",
		},
		{
			name: "resulting fact reference",
			mutate: func(value map[string]any) {
				value["proposal_dispositions"].([]any)[0].(map[string]any)["resulting_fact_id"] = "fact:summary:0000000000000000000000000000000000000000000000000000000000000000"
			},
			want: "references missing resulting fact",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneJSON(t, base)
			test.mutate(value)
			mutated, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			fsys := fstest.MapFS{"document.json": {Data: mutated}}
			doc, parseErr := documentdata.Parse(fsys, "document.json", "policy", "rhoai-test")
			if doc != nil || parseErr == nil || !strings.Contains(parseErr.Error(), "shared analyzer validator") || !strings.Contains(parseErr.Error(), test.want) {
				t.Fatalf("Parse() = %#v, %v; want shared validation error containing %q", doc, parseErr, test.want)
			}
		})
	}
}

func readJSONFixture(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cloneJSON(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

type legacyFields struct {
	Name, Repository, Branch, Version, Languages, DeployType, Purpose, PurposeFull string
	Metadata                                                                       map[string]string
	Components                                                                     []types.ArchComponent
	CRDs                                                                           []types.CRD
	Endpoints                                                                      []types.Endpoint
	GRPCServices                                                                   []types.GRPCService
	ExternalDeps, InternalDeps                                                     []types.Dependency
	Services                                                                       []types.Service
	Ingresses                                                                      []types.Ingress
	Egresses                                                                       []types.Egress
	RBACRoles                                                                      []types.RBACRole
}

func legacyProjection(doc *types.ComponentDoc) legacyFields {
	return legacyFields{
		Name: doc.Name, Repository: doc.Repository, Branch: doc.Branch, Version: doc.Version,
		Languages: doc.Languages, DeployType: doc.DeployType, Purpose: doc.Purpose, PurposeFull: doc.PurposeFull,
		Metadata: doc.Metadata, Components: doc.Components, CRDs: doc.CRDs, Endpoints: doc.Endpoints,
		GRPCServices: doc.GRPCServices, ExternalDeps: doc.ExternalDeps, InternalDeps: doc.InternalDeps,
		Services: doc.Services, Ingresses: doc.Ingresses, Egresses: doc.Egresses, RBACRoles: doc.RBACRoles,
	}
}
