package renderer

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/model"
	"github.com/jctanner/arch-analyzer/internal/normalize"
	"github.com/jctanner/arch-analyzer/internal/structured"
)

const acceptedAnalyzerFixture = `{
  "component":"policy",
  "repo":"https://github.com/praxis-proxy/policy.git",
  "commit_sha":"0123456789abcdef",
  "extracted_at":"2026-09-06T12:00:00Z",
  "analyzer_version":"0.1.0-dev",
  "schema_version":"1",
  "summary":"Praxis policy evaluation service.",
  "source_components":[{"name":"policy-api","type":"service","purpose":"Evaluates policy","source":"cmd/server.go:1-40"}],
  "rbac":{
    "cluster_roles":[{"name":"policy-reader","source":"config/rbac.yaml:1-40","rules":[
      {"apiGroups":["example.io"],"resources":["policies"],"verbs":["get","list"]},
      {"apiGroups":[],"resources":[],"nonResourceURLs":["/metrics"],"verbs":["get"]},
      {"apiGroups":["example.io"],"resources":["policies/status"],"nonResourceURLs":["/healthz"],"verbs":["get"]},
      {"apiGroups":[],"resources":[],"verbs":["get"]}
    ]}],
    "roles":[],"cluster_role_bindings":[],"role_bindings":[]
  },
  "security_evidence":[{"kind":"fips-posture","target":"policy-api","detail":"FIPS mode is explicitly unresolved","status":"not-extracted","source":"cmd/server.go:20-24"}],
  "data_coverage":{"source":"complete"}
}`

func TestAcceptedDocumentRendersByteIdenticalToExistingRenderer(t *testing.T) {
	var input model.Input
	if err := json.Unmarshal([]byte(acceptedAnalyzerFixture), &input); err != nil {
		t.Fatal(err)
	}
	componentMap := &model.ComponentMap{Components: map[string]model.ComponentEntry{
		"praxis-policy": {RepoOrg: "praxis-proxy", RepoName: "policy", RepoURL: "https://github.com/praxis-proxy/policy"},
	}}
	legacy := normalize.Input(input, normalize.Options{Distribution: "RHOAI", GeneratedBy: "parity-test", ComponentMap: componentMap})
	var baseline bytes.Buffer
	if err := Markdown(&baseline, legacy); err != nil {
		t.Fatal(err)
	}

	accepted, err := structured.Normalize(strings.NewReader(acceptedAnalyzerFixture), structured.Options{
		Distribution: "RHOAI", GeneratedBy: "parity-test", ComponentMap: componentMap,
		VersionScope: "rhoai-3.6-ea.2", IntegrationStatus: "not-integrated",
		Aliases: []string{"praxis-proxy/policy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := AcceptedMarkdown(&actual, accepted); err != nil {
		t.Fatal(err)
	}
	if baseline.String() != actual.String() {
		t.Fatalf("accepted renderer differs from existing renderer\n--- baseline ---\n%s\n--- accepted ---\n%s", baseline.String(), actual.String())
	}
	if accepted.Identity.Component != "praxis-policy" || accepted.Identity.SourceComponent != "policy" ||
		accepted.Identity.Repository != "https://github.com/praxis-proxy/policy.git" || accepted.Identity.IntegrationStatus != "not-integrated" {
		t.Fatalf("prefixed/source identity not preserved: %#v", accepted.Identity)
	}
	wantAliases := []string{"policy", "praxis-proxy/policy"}
	if !reflect.DeepEqual(accepted.Identity.Aliases, wantAliases) {
		t.Fatalf("aliases=%v, want %v", accepted.Identity.Aliases, wantAliases)
	}
	for _, row := range []string{
		"| policy-reader | example.io | policies |  | get, list |",
		"| policy-reader |  |  | /metrics | get |",
		"| policy-reader | example.io | policies/status | /healthz | get |",
		"| policy-reader |  |  |  | get |",
	} {
		if !strings.Contains(actual.String(), row) {
			t.Errorf("missing RBAC variant %q\n%s", row, actual.String())
		}
	}
}

func TestAcceptedRendererOwnsConditionalHeadingsAndFIPSParent(t *testing.T) {
	sections := []model.StructuredSection{
		section("aipcc-ecosystems-use", "not-applicable", []model.StructuredContentBlock{{Type: "paragraph", Text: "No applicable package installation was extracted."}}),
		section("sub-component-details", "documented", []model.StructuredContentBlock{{Type: "list", Items: []string{"policy-api is the extracted runtime component."}}}),
		section("deployment-manifests", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "The deployment is sourced from the selected overlay."}}),
		withUncertainty(section("security.fips-compliance", "unresolved", []model.StructuredContentBlock{{Type: "paragraph", Text: "FIPS support remains unresolved from available analyzer evidence."}}), "Runtime FIPS mode was not extracted."),
		section("security.build-hermeticity", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "Build inputs are explicitly pinned."}}),
		withUncertainty(section("multi-tenancy", "unresolved", []model.StructuredContentBlock{{Type: "paragraph", Text: "Namespace isolation remains unverified."}}), "Tenant isolation was not extracted."),
	}
	accepted, err := structured.Normalize(strings.NewReader(acceptedAnalyzerFixture), structured.Options{Sections: sections})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := AcceptedMarkdown(&output, accepted); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, heading := range []string{
		"## AIPCC Ecosystems Use", "## Sub-Component Details", "## Deployment Manifests",
		"### FIPS Compliance", "### Build Hermeticity", "## Multi-Tenancy",
	} {
		if strings.Count(text, heading+"\n") != 1 {
			t.Errorf("heading %q not rendered exactly once\n%s", heading, text)
		}
	}
	security := strings.Index(text, "## Security\n")
	fips := strings.Index(text, "### FIPS Compliance\n")
	multi := strings.Index(text, "## Multi-Tenancy\n")
	if security < 0 || fips < security || multi < fips || strings.Contains(text, "\n## FIPS Compliance\n") {
		t.Fatalf("FIPS hierarchy/order invalid: security=%d fips=%d multi=%d\n%s", security, fips, multi, text)
	}
	if !strings.Contains(text, "**Evidence**: cmd/server.go:20-24@0123456789abcdef") {
		t.Fatalf("typed section evidence missing\n%s", text)
	}
	for _, want := range []string{
		"### FIPS Compliance\n\n**Status**: unresolved\n\n**Uncertainty**: Runtime FIPS mode was not extracted.",
		"## Multi-Tenancy\n\n**Status**: unresolved\n\n**Uncertainty**: Tenant isolation was not extracted.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("unresolved section metadata missing %q\n%s", want, text)
		}
	}
}

func TestAcceptedRendererRejectsStructuralContentAndMisorderedSections(t *testing.T) {
	tests := []struct {
		name     string
		sections []model.StructuredSection
		want     string
	}{
		{"heading injection", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "## Escaped Security"}})}, "headings"},
		{"raw table", []model.StructuredSection{section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "a | b"}})}, "table"},
		{"fence", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "```go"}})}, "fenced code"},
		{"tilde fence", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "~~~go"}})}, "fenced code"},
		{"thematic break", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "---"}})}, "thematic break"},
		{"indented code", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "    code"}})}, "indented code"},
		{"tab code", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "\tcode"}})}, "control characters"},
		{"ordered list", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "1. item"}})}, "list syntax"},
		{"unordered list", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "- item"}})}, "list syntax"},
		{"block quote", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "> quote"}})}, "block quotes"},
		{"raw link", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "[label](javascript:alert(1))"}})}, "links"},
		{"autolink", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "<https://example.com>"}})}, "raw HTML"},
		{"raw HTML", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "<details>hidden</details>"}})}, "raw HTML"},
		{"paragraph escape", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "unsafe\x1btext"}})}, "control characters"},
		{"list unicode separator", []model.StructuredSection{section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "list", Items: []string{"unsafe\u2028text"}}})}, "control characters"},
		{"table delete control", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "bad\x7ftext", "source.go"}, Evidence: []model.StructuredEvidenceRef{{Path: "source.go", Revision: "rev"}}}}}})}, "control characters"},
		{"uncertainty next-line", []model.StructuredSection{withUncertainty(section("multi-tenancy", "unresolved", []model.StructuredContentBlock{{Type: "paragraph", Text: "Unresolved."}}), "unsafe\u0085text")}, "control characters"},
		{"table cell structure", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "bad | split", "source.go"}, Evidence: []model.StructuredEvidenceRef{{Path: "source.go", Revision: "rev"}}}}}})}, "table syntax"},
		{"section evidence path injection", []model.StructuredSection{withEvidencePath(section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "Evidence must remain inline."}}), "cmd/server.go\n```")}, "single line without control"},
		{"section evidence revision injection", []model.StructuredSection{withEvidenceRevision(section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "Evidence must remain inline."}}), "revision\n```")}, "single line without control"},
		{"table evidence path injection", []model.StructuredSection{section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "multi-tenancy.tenant-model", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "Value", "Source"}, Evidence: []model.StructuredEvidenceRef{{Path: "source.go\n```", Revision: "rev"}}}}}})}, "single line without control"},
		{"table evidence revision injection", []model.StructuredSection{section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "multi-tenancy.tenant-model", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "Value", "Source"}, Evidence: []model.StructuredEvidenceRef{{Path: "source.go", Revision: "rev\n```"}}}}}})}, "single line without control"},
		{"wrong table width", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "Value"}, Evidence: []model.StructuredEvidenceRef{{Path: "source.go", Revision: "rev"}}}}}})}, "cells, want 3"},
		{"empty table", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{}}})}, "requires at least one row"},
		{"missing row evidence", []model.StructuredSection{section("security.fips-compliance", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{{Cells: []string{"Aspect", "Value", "Source"}, Evidence: nil}}}})}, "requires evidence"},
		{"wrong section table", []model.StructuredSection{section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{}}})}, "unsupported table"},
		{"missing evidence revision", []model.StructuredSection{withoutEvidenceRevision(section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "Evidence must be revision-bound."}}))}, "missing evidence revision"},
		{"misordered", []model.StructuredSection{section("multi-tenancy", "not-applicable", []model.StructuredContentBlock{}), section("security.fips-compliance", "not-applicable", []model.StructuredContentBlock{})}, "not in registry order"},
		{"unknown", []model.StructuredSection{section("security.fips", "not-applicable", []model.StructuredContentBlock{})}, "unsupported structured section"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := structured.Normalize(strings.NewReader(acceptedAnalyzerFixture), structured.Options{Sections: test.sections})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestAcceptedRendererRendersRegistryOwnedBaselineTablesAndSafeInline(t *testing.T) {
	sections := []model.StructuredSection{
		section("security.fips-compliance", "documented", []model.StructuredContentBlock{
			{Type: "paragraph", Text: "Uses **strict FIPS**, `crypto/tls`, and slice[0] < limit."},
			{Type: "table", TableID: "security.fips-build-time", Rows: []model.StructuredTableRow{{
				Cells:    []string{"**Build flags**", "CGO_ENABLED=1", "Dockerfile.konflux:19"},
				Evidence: []model.StructuredEvidenceRef{{Path: "Dockerfile.konflux", StartLine: 19, EndLine: 19, Revision: "0123456789abcdef"}},
			}}},
		}),
		section("security.build-hermeticity", "documented", []model.StructuredContentBlock{{
			Type: "table", TableID: "security.build-hermeticity", Rows: []model.StructuredTableRow{{
				Cells:    []string{"Language deps", "go.sum", "Yes", "go mod", "go.sum"},
				Evidence: []model.StructuredEvidenceRef{{Path: "go.sum", Revision: "0123456789abcdef"}},
			}},
		}}),
		section("multi-tenancy", "documented", []model.StructuredContentBlock{{
			Type: "table", TableID: "multi-tenancy.tenant-model", Rows: []model.StructuredTableRow{{
				Cells:    []string{"Tenant boundary", "per-namespace", "api/types.go:12"},
				Evidence: []model.StructuredEvidenceRef{{Path: "api/types.go", StartLine: 12, EndLine: 12, Revision: "0123456789abcdef"}},
			}},
		}}),
	}
	accepted, err := structured.Normalize(strings.NewReader(acceptedAnalyzerFixture), structured.Options{Sections: sections})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := AcceptedMarkdown(&output, accepted); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"#### Build-Time FIPS (check-payload gate)",
		"| Aspect | Value | Source |",
		"| **Build flags** | CGO_ENABLED=1 | Dockerfile.konflux:19 |",
		"| Layer | Lock File | Present | Tool | Source |",
		"### Tenant Model",
		"**Table Evidence**: Dockerfile.konflux:19@0123456789abcdef",
		"Uses **strict FIPS**, `crypto/tls`, and slice\\[0\\] &lt; limit.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q\n%s", want, text)
		}
	}
}

func TestPlannedAndSupportSectionsAreRetainedButNotRendered(t *testing.T) {
	for _, claimClass := range []string{"planned", "support"} {
		t.Run(claimClass, func(t *testing.T) {
			content := section("multi-tenancy", "documented", []model.StructuredContentBlock{{Type: "paragraph", Text: "Non-implementation tenant guidance."}})
			content.Authority = model.StructuredAuthority{Kind: "overlay", Origin: "roadmap", ClaimClass: claimClass}
			accepted, err := structured.Normalize(strings.NewReader(acceptedAnalyzerFixture), structured.Options{Sections: []model.StructuredSection{content}})
			if err != nil {
				t.Fatal(err)
			}
			if len(accepted.Sections) != 1 || accepted.Sections[0].Authority.ClaimClass != claimClass {
				t.Fatalf("section attribution not retained: %#v", accepted.Sections)
			}
			var output bytes.Buffer
			if err := AcceptedMarkdown(&output, accepted); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "## Multi-Tenancy") || strings.Contains(output.String(), "tenant guidance") {
				t.Fatalf("%s section rendered as implementation\n%s", claimClass, output.String())
			}
		})
	}
}

func section(id, status string, blocks []model.StructuredContentBlock) model.StructuredSection {
	evidence := []model.StructuredEvidenceRef{{Path: "cmd/server.go", StartLine: 20, EndLine: 24, Revision: "0123456789abcdef"}}
	return model.StructuredSection{
		ID: id, Status: status,
		Authority: model.StructuredAuthority{Kind: "model", Origin: "response-01", ClaimClass: "implementation"},
		Blocks:    blocks, Evidence: evidence,
	}
}

func withUncertainty(section model.StructuredSection, uncertainty string) model.StructuredSection {
	section.Uncertainty = uncertainty
	return section
}

func withoutEvidenceRevision(section model.StructuredSection) model.StructuredSection {
	section.Evidence[0].Revision = ""
	return section
}

func withEvidencePath(section model.StructuredSection, path string) model.StructuredSection {
	section.Evidence[0].Path = path
	return section
}

func withEvidenceRevision(section model.StructuredSection, revision string) model.StructuredSection {
	section.Evidence[0].Revision = revision
	return section
}

func TestSafeEvidenceInlineDefensivelyRemovesLineStructure(t *testing.T) {
	actual := safeEvidenceInline("cmd/server.go\n```@revision\u2028~~~")
	if strings.ContainsAny(actual, "\r\n\u2028\u2029") || strings.Contains(actual, "```") {
		t.Fatalf("safeEvidenceInline() left structural content in %q", actual)
	}
	if !strings.Contains(actual, "\\`\\`\\`") {
		t.Fatalf("safeEvidenceInline() did not neutralize backticks: %q", actual)
	}
}
