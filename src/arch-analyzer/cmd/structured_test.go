package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/structured"
)

func TestNormalizeAndRenderDocumentCLIByteParity(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	componentMap := "../../../tests/fixtures/structured_component/component-map-praxis.json"
	directory := t.TempDir()
	documentPath := filepath.Join(directory, "document.json")
	acceptedMarkdown := filepath.Join(directory, "accepted.md")
	legacyMarkdown := filepath.Join(directory, "legacy.md")

	if err := Execute([]string{
		"normalize", "--input", fixture, "--output", documentPath,
		"--component-map", componentMap, "--distribution", "RHOAI",
		"--generated-by", "cli-parity", "--version-scope", "rhoai-3.6-ea.2",
		"--integration-status", "not-integrated", "--aliases", "praxis-proxy/policy",
	}); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{"render-document", "--input", documentPath, "--output", acceptedMarkdown}); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{
		"render", "--input", fixture, "--output", legacyMarkdown,
		"--component-map", componentMap, "--distribution", "RHOAI", "--generated-by", "cli-parity",
	}); err != nil {
		t.Fatal(err)
	}
	accepted, err := os.ReadFile(acceptedMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(legacyMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if string(accepted) != string(legacy) {
		t.Fatalf("CLI byte parity failed\n--- accepted ---\n%s\n--- legacy ---\n%s", accepted, legacy)
	}
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"component": "praxis-policy"`, `"source_component": "policy"`,
		`"repository": "https://github.com/praxis-proxy/policy.git"`,
		`"integration_status": "not-integrated"`, `"non_resource_urls": "/metrics"`,
	} {
		if !strings.Contains(string(document), want) {
			t.Errorf("document missing %s", want)
		}
	}
}

func TestNormalizeCLIAcceptsOnlyValidatedTrustedReuseRecord(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	directory := t.TempDir()
	reusePath := filepath.Join(directory, "reuse.json")
	documentPath := filepath.Join(directory, "document.json")
	hash := "sha256:" + strings.Repeat("a", 64)
	record := structured.ReuseRecord{
		SchemaVersion: "1.0.0", PriorSnapshotID: "snapshot-1",
		PriorPlatform: "rhoai-old", TargetPlatform: "rhoai-target",
		PriorDocumentHash: hash, OriginalSynthesisHash: hash,
		ResponseIdentity: "response-1", TargetExactInput: hash,
		TargetSemanticInput: hash, ComparisonHash: hash,
		Reason:             "verified semantic and dependency match",
		RevalidationChecks: []string{"current_acceptance_rules", "document_schema"},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reusePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{
		"normalize", "--input", fixture, "--version-scope", "rhoai-target",
		"--reuse-record", reusePath, "--output", documentPath,
	}); err != nil {
		t.Fatal(err)
	}
	documentFile, err := os.Open(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	document, err := structured.Decode(documentFile)
	closeErr := documentFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if document.Reuse == nil || !reflect.DeepEqual(*document.Reuse, record) {
		t.Fatalf("reuse record=%#v, want %#v", document.Reuse, record)
	}

	if err := os.WriteFile(reusePath, append(raw, []byte(` {"extra":true}`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	err = Execute([]string{
		"normalize", "--input", fixture, "--version-scope", "rhoai-target",
		"--reuse-record", reusePath,
	})
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing reuse record error=%v", err)
	}
}

func TestNormalizeCLIRendersConditionalSectionsWithFIPSUnderSecurity(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	sections := "../../../tests/fixtures/structured_component/sections-all-conditional.json"
	directory := t.TempDir()
	documentPath := filepath.Join(directory, "document.json")
	markdownPath := filepath.Join(directory, "component.md")
	if err := Execute([]string{
		"normalize", "--input", fixture, "--sections", sections,
		"--section-origin-kind", "model", "--section-origin-id", "fixture-response",
		"--section-claim-class", "implementation", "--output", documentPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Execute([]string{"render-document", "--input", documentPath, "--output", markdownPath}); err != nil {
		t.Fatal(err)
	}
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(markdown)
	security := strings.Index(text, "## Security\n")
	fips := strings.Index(text, "### FIPS Compliance\n")
	dataFlows := strings.Index(text, "## Data Flows\n")
	if security < 0 || fips < security || dataFlows < fips || strings.Contains(text, "\n## FIPS Compliance\n") {
		t.Fatalf("FIPS is not a Security child\n%s", text)
	}
	for _, want := range []string{
		"### FIPS Compliance\n\n**Status**: unresolved\n\n**Uncertainty**: Runtime FIPS mode was not extracted.",
		"## Multi-Tenancy\n\n**Status**: unresolved\n\n**Uncertainty**: Tenant isolation was not extracted.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("round-tripped conditional fixture missing %q\n%s", want, text)
		}
	}
	documentRaw, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(documentRaw, &document); err != nil {
		t.Fatal(err)
	}
	sectionsValue := document["sections"].([]any)
	unresolved := map[string]string{}
	for _, rawSection := range sectionsValue {
		section := rawSection.(map[string]any)
		if section["status"] == "unresolved" {
			unresolved[section["id"].(string)] = section["uncertainty"].(string)
		}
		for _, rawBlock := range section["blocks"].([]any) {
			block := rawBlock.(map[string]any)
			if block["type"] == "table" {
				if len(block["rows"].([]any)) == 0 {
					t.Fatalf("encoded table has no rows: %#v", block)
				}
				continue
			}
			if _, exists := block["table_id"]; exists {
				t.Fatalf("non-table block emitted table_id: %#v", block)
			}
			if _, exists := block["rows"]; exists {
				t.Fatalf("non-table block emitted rows: %#v", block)
			}
		}
	}
	wantUnresolved := map[string]string{
		"security.fips-compliance": "Runtime FIPS mode was not extracted.",
		"multi-tenancy":            "Tenant isolation was not extracted.",
	}
	if !reflect.DeepEqual(unresolved, wantUnresolved) {
		t.Fatalf("JSON round-trip unresolved sections=%#v, want %#v", unresolved, wantUnresolved)
	}
}

func TestNormalizeCLIRejectsSectionAndTableEvidenceLineInjection(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	originalPath := "../../../tests/fixtures/structured_component/sections-all-conditional.json"
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func([]map[string]any)
	}{
		{"section path", func(sections []map[string]any) {
			sections[0]["evidence"].([]any)[0].(map[string]any)["path"] = "source.go\n```"
		}},
		{"section revision", func(sections []map[string]any) {
			sections[0]["evidence"].([]any)[0].(map[string]any)["revision"] = "rev\n```"
		}},
		{"table path", func(sections []map[string]any) {
			sections[0]["blocks"].([]any)[1].(map[string]any)["rows"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["path"] = "source.go\n```"
		}},
		{"table revision", func(sections []map[string]any) {
			sections[0]["blocks"].([]any)[1].(map[string]any)["rows"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["revision"] = "rev\n```"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sections []map[string]any
			if err := json.Unmarshal(original, &sections); err != nil {
				t.Fatal(err)
			}
			test.mutate(sections)
			raw, err := json.Marshal(sections)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "sections.json")
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			err = Execute([]string{
				"normalize", "--input", fixture, "--sections", path,
				"--section-origin-kind", "model", "--section-origin-id", "fixture-response",
				"--section-claim-class", "implementation",
			})
			if err == nil || !strings.Contains(err.Error(), "single line without control") {
				t.Fatalf("Execute() error=%v", err)
			}
		})
	}
}

func TestRenderDocumentCLIRejectsFactEvidenceLineInjection(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	for _, field := range []string{"path", "revision"} {
		t.Run(field, func(t *testing.T) {
			directory := t.TempDir()
			documentPath := filepath.Join(directory, "document.json")
			if err := Execute([]string{"normalize", "--input", fixture, "--output", documentPath}); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(documentPath)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			var reference map[string]any
			for _, rawFact := range document["facts"].([]any) {
				fact := rawFact.(map[string]any)
				evidence := fact["evidence"].([]any)
				if len(evidence) > 0 {
					reference = evidence[0].(map[string]any)
					break
				}
			}
			if reference == nil {
				t.Fatal("fixture document has no fact evidence")
			}
			reference[field] = "injected\n```"
			injected, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(documentPath, injected, 0o600); err != nil {
				t.Fatal(err)
			}
			err = Execute([]string{"render-document", "--input", documentPath})
			if err == nil || !strings.Contains(err.Error(), "single line without control") {
				t.Fatalf("Execute() error=%v", err)
			}
		})
	}
}

func TestNormalizeCLIRejectsUnknownRequiredFact(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/invalid-unknown-fact.json"
	err := Execute([]string{"normalize", "--input", fixture, "--output", filepath.Join(t.TempDir(), "document.json")})
	if err == nil || !strings.Contains(err.Error(), "unsupported analyzer fact") {
		t.Fatalf("Execute() error=%v", err)
	}
}

func TestNormalizeCLIRejectsSectionSelfAttribution(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	sectionsPath := filepath.Join(t.TempDir(), "sections.json")
	selfAttributed := `[{
	  "id":"multi-tenancy","status":"documented",
	  "authority":{"kind":"human-correction","origin":"self","claim_class":"correction"},
	  "blocks":[{"type":"paragraph","text":"Self-attributed content."}],
	  "evidence":[{"path":"cmd/server.go","revision":"0123456789abcdef"}]
	}]`
	if err := os.WriteFile(sectionsPath, []byte(selfAttributed), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Execute([]string{
		"normalize", "--input", fixture, "--sections", sectionsPath,
		"--section-origin-kind", "model", "--section-origin-id", "response-01",
		"--section-claim-class", "implementation",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("self-attributed sections error=%v", err)
	}
}

func TestNormalizeCLIRequiresSeparateTrustedPolicyAndAppliesHumanDecision(t *testing.T) {
	fixture := "../../../tests/fixtures/structured_component/analyzer-rbac-praxis.json"
	directory := t.TempDir()
	basePath := filepath.Join(directory, "base.json")
	if err := Execute([]string{"normalize", "--input", fixture, "--version-scope", "rhoai-test", "--output", basePath}); err != nil {
		t.Fatal(err)
	}
	baseFile, err := os.Open(basePath)
	if err != nil {
		t.Fatal(err)
	}
	base, err := structured.Decode(baseFile)
	closeErr := baseFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	var target structured.Fact
	for _, fact := range base.Facts {
		if fact.Type == "summary" {
			target = fact
			break
		}
	}
	if target.ID == "" {
		t.Fatal("summary fact not found")
	}
	patch := structured.PatchSet{
		SchemaVersion: structured.PatchSchemaVersion, PatchID: "model-proposal",
		BundleFingerprint: base.AnalyzerInput.BundleFingerprint,
		Operations: []structured.PatchOperation{{
			OperationID: "update-summary", Action: "update", FactType: "summary", TargetFactID: target.ID,
			Value:    json.RawMessage(`"Reviewed policy evaluation service."`),
			Evidence: []structured.EvidenceRef{{Path: "cmd/server.go", StartLine: 1, EndLine: 40, Revision: "0123456789abcdef"}},
			Reason:   "Model proposal for human review.",
		}},
	}
	patchPath := filepath.Join(directory, "proposal.json")
	patchRaw, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patchPath, patchRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	err = Execute([]string{"normalize", "--input", fixture, "--version-scope", "rhoai-test", "--patch", patchPath, "--output", filepath.Join(directory, "unauthorized.json")})
	if err == nil || !strings.Contains(err.Error(), "no trusted assembly policy") {
		t.Fatalf("CLI unauthorized proposal error=%v", err)
	}
	fingerprint, err := structured.ProposalFingerprint(patch)
	if err != nil {
		t.Fatal(err)
	}
	policy := structured.AssemblyPolicy{
		SchemaVersion: structured.PolicySchemaVersion, PolicyID: "human-policy", PatchID: patch.PatchID,
		ProposalFingerprint: fingerprint, BundleFingerprint: patch.BundleFingerprint, VersionScope: "rhoai-test",
		Origin:    structured.PatchOrigin{Kind: "model", ID: "response-01", ClaimClass: "correction"},
		Authority: structured.PatchAuthority{Actor: "orchestrator", AllowedFactTypes: []string{"summary"}},
		Decisions: []structured.AssemblyDecision{{
			DecisionID: "human-decision", OperationID: "update-summary", Decision: "accept",
			DecidedBy: "reviewer@example", Reason: "Explicit human approval after evidence review.",
		}},
	}
	policyPath := filepath.Join(directory, "policy.json")
	policyRaw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, policyRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	acceptedPath := filepath.Join(directory, "accepted.json")
	if err := Execute([]string{
		"normalize", "--input", fixture, "--version-scope", "rhoai-test",
		"--patch", patchPath, "--assembly-policy", policyPath, "--output", acceptedPath,
	}); err != nil {
		t.Fatal(err)
	}
	acceptedFile, err := os.Open(acceptedPath)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := structured.Decode(acceptedFile)
	closeErr = acceptedFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if accepted.RenderingView.Purpose != "Reviewed policy evaluation service." ||
		len(accepted.Dispositions) != 1 || accepted.Dispositions[0].DecidedBy != "reviewer@example" {
		t.Fatalf("trusted CLI decision was not applied and retained: %#v", accepted.Dispositions)
	}
}
