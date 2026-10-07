package embeddeddata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analyzerdocument "github.com/jctanner/arch-analyzer/pkg/document"
)

func TestStageIncludesAcceptedAndLegacyArtifactsOnly(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "architecture")
	destination := filepath.Join(root, "staged")
	overlays := filepath.Join(root, "overlays")
	mustWrite(t, filepath.Join(source, "rhoai-1", "component.md"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component.json"))
	mustCopy(t, filepath.Join("..", "documentdata", "testdata", "rhoai.next", "praxis-policy", "document.json"), filepath.Join(source, "rhoai-1", "component", "document.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", "analyzer.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", "synthesis.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", "diagrams", "component.svg"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", ".analyzer", "component-architecture.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", ".analyzer", "analyzer_synthesis_context.md"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", ".generation", "candidate.md"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "component", "attempts", "01", "response.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "run-metadata", "document.json"))
	mustWrite(t, filepath.Join(source, "rhoai-1", "contracts", "schemas", "component", "kind.json"))
	mustWrite(t, filepath.Join(overlays, "README.md"))
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("rhoai-1", filepath.Join(source, "current")); err != nil {
		t.Fatal(err)
	}
	if err := Stage(source, destination, overlays); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"rhoai-1/component.md", "rhoai-1/component.json",
		"rhoai-1/component/document.json", "rhoai-1/component/analyzer.json", "rhoai-1/component/synthesis.json",
		"rhoai-1/component/diagrams/component.svg", "rhoai-1/component/.analyzer/component-architecture.json",
		"rhoai-1/contracts/schemas/component/kind.json", "overlays/README.md", "symlinks.json",
	} {
		if _, err := os.Stat(filepath.Join(destination, path)); err != nil {
			t.Errorf("required staged path %s: %v", path, err)
		}
	}
	for _, path := range []string{
		"rhoai-1/component/.generation/candidate.md",
		"rhoai-1/component/attempts/01/response.json",
		"rhoai-1/component/.analyzer/analyzer_synthesis_context.md",
		"rhoai-1/run-metadata/document.json",
		"current",
	} {
		if _, err := os.Stat(filepath.Join(destination, path)); !os.IsNotExist(err) {
			t.Errorf("excluded path %s was staged", path)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(destination, "symlinks.json"))
	if err != nil || string(manifest) != "{\"current\":\"rhoai-1\"}\n" {
		t.Fatalf("alias manifest = %q, %v", manifest, err)
	}
}

func TestStageRejectsPartialOrInvalidAcceptedSnapshots(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"partial":             {"component/analyzer.json": `{}`},
		"interrupted partial": {"component/.document.json.next": `{}`},
		"invalid":             {"component/document.json": `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "architecture")
			destination := filepath.Join(root, "staged")
			for path, body := range files {
				full := filepath.Join(source, "rhoai-1", path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := Stage(source, destination, ""); err == nil {
				t.Fatal("Stage accepted an incomplete or invalid structured snapshot")
			}
		})
	}
}

func TestStageValidatesCompletePublishedSnapshot(t *testing.T) {
	document, analyzer, synthesis, markdown := publishedStageFixture(t)
	for _, test := range []struct {
		name      string
		synthesis []byte
		wantError bool
	}{
		{name: "complete", synthesis: synthesis},
		{name: "invalid synthesis", synthesis: []byte(`{}`), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "architecture")
			destination := filepath.Join(root, "staged")
			mustWriteBytes(t, filepath.Join(source, "rhoai.next/typed-only/document.json"), document)
			mustWriteBytes(t, filepath.Join(source, "rhoai.next/typed-only/analyzer.json"), analyzer)
			mustWriteBytes(t, filepath.Join(source, "rhoai.next/typed-only/synthesis.json"), test.synthesis)
			mustWriteBytes(t, filepath.Join(source, "rhoai.next/typed-only.md"), markdown)
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatal(err)
			}
			err := Stage(source, destination, "")
			if test.wantError && err == nil {
				t.Fatal("Stage accepted invalid published synthesis")
			}
			if !test.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func publishedStageFixture(t *testing.T) ([]byte, []byte, []byte, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "documentdata", "testdata", "rhoai.next", "typed-only", "document.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	identity := document["identity"].(map[string]any)
	producers := document["producers"].(map[string]any)
	analyzerInput := document["analyzer_input"].(map[string]any)
	analyzerValue := map[string]any{
		"schema_version": "1", "component": identity["source_component"],
		"repo": identity["repository"], "commit_sha": identity["source_revision"],
		"extracted_at":     analyzerInput["extracted_at"],
		"analyzer_version": producers["analyzer_version"],
	}
	analyzer, err := json.Marshal(analyzerValue)
	if err != nil {
		t.Fatal(err)
	}
	bundleFingerprint := stageHash(analyzer)
	analyzerInput["bundle_fingerprint"] = bundleFingerprint
	hash := "sha256:" + strings.Repeat("a", 64)
	synthesisValue := map[string]any{
		"schema_version": "1.0.0", "state": "deterministic-only", "route": "structured-component/v1",
		"input_bundle_identity": hash, "context_identity": hash,
		"reuse_eligibility": map[string]any{"available": false, "reason": "offline fixture", "unobserved_context": []any{}},
		"requested":         map[string]any{"harness": "none", "model": "none", "settings": map[string]any{}},
		"reported":          map[string]any{"models": []any{}, "settings": []any{}, "auxiliary_models": []any{}},
		"calls":             map[string]any{"total": 0, "initial": 0, "evidence_followup": 0, "repair": 0, "limits": map[string]any{"total": 0, "evidence_followup": 0, "repair": 0}},
		"responses":         []any{}, "resolution_provenance": []any{}, "observations": map[string]any{},
		"justifications": []any{}, "accepted_response_identity": nil,
		"diagnostics": []any{map[string]any{"code": "deterministic-only", "detail": "offline fixture"}},
	}
	synthesis, err := json.Marshal(synthesisValue)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := map[string]any{"state": "unavailable", "reason": "deterministic-only"}
	document["schema_version"] = "1.1.0"
	document["publication"] = map[string]any{
		"contract": "structured-component-publication/v1", "snapshot_id": hash,
		"analyzer": map[string]any{
			"content_hash": stageHash(analyzer), "bundle_fingerprint": bundleFingerprint,
			"schema_version": "1", "source_component": identity["source_component"],
			"repository": identity["repository"], "source_revision": identity["source_revision"],
			"analyzer_version": producers["analyzer_version"], "producer_build_identity": hash,
		},
		"synthesis": map[string]any{
			"content_hash": stageHash(synthesis), "state": "deterministic-only",
			"input_bundle_identity": hash, "current_evidence_bundle_identity": stageValueHash(unavailable),
			"original_evidence_bundle_identity": stageValueHash(unavailable), "producing_model_eligible": false,
		},
		"accepted_inputs": map[string]any{
			"run_record": unavailable, "current_evidence_bundle": unavailable, "original_evidence_bundle": unavailable,
		},
		"markdown": map[string]any{"path": "typed-only.md", "renderer_version": "arch-analyzer-markdown/v1"},
		"diagram":  map[string]any{"state": "unavailable"},
	}
	documentData, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := analyzerdocument.RenderMarkdownJSON(documentData)
	if err != nil {
		t.Fatal(err)
	}
	return documentData, analyzer, synthesis, markdown
}

func stageHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stageValueHash(value any) string {
	raw, _ := json.Marshal(value)
	return stageHash(raw)
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteBytes(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustCopy(t *testing.T, source, destination string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
