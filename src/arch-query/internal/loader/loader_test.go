package loader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	analyzerdocument "github.com/jctanner/arch-analyzer/pkg/document"
)

func TestLoadVersionMergesComponentAnalyzerArtifacts(t *testing.T) {
	fsys := fstest.MapFS{
		"rhoai.next/example.md": {
			Data: []byte(`# Component: example

## Overview

Example component.
`),
		},
		"rhoai.next/example/.analyzer/component-architecture.json": {
			Data: []byte(`{
				"component": "example",
				"webhooks": [
					{
						"name": "vexample.kb.io",
						"type": "validating",
						"path": "/validate-example",
						"rules": [
							{"apiGroups": ["example.io"], "apiVersions": ["v1"], "resources": ["examples"], "operations": ["CREATE"]}
						]
					}
				],
				"platform_webhooks": [{"component": "example", "webhook": "vexample.kb.io"}],
				"controller_watches": [{"type": "Owns", "gvk": "apps/v1.Deployment", "controller": "example", "source": "controllers/example.go"}]
			}`),
		},
	}

	data, err := LoadVersion(fsys, nil, "rhoai.next")
	if err != nil {
		t.Fatalf("LoadVersion: %v", err)
	}
	doc := data.Components["example"]
	if doc == nil {
		t.Fatal("example component was not loaded")
	}
	if len(doc.Webhooks) != 1 {
		t.Fatalf("expected analyzer webhook to be merged, got %d", len(doc.Webhooks))
	}
	if len(doc.PlatformWebhooks) != 1 {
		t.Fatalf("expected platform webhook ref to be merged, got %d", len(doc.PlatformWebhooks))
	}
	if len(doc.ControllerWatches) != 1 {
		t.Fatalf("expected controller watch to be merged, got %d", len(doc.ControllerWatches))
	}
}

func TestLoadVersionAcceptedDocumentIsAuthoritative(t *testing.T) {
	document := readFixture(t, "../documentdata/testdata/rhoai.next/typed-only/document.json")
	fsys := fstest.MapFS{
		"rhoai.next/typed-only/document.json": {Data: document},
		"rhoai.next/typed-only.md": {Data: []byte(`# Component: wrong-markdown-name

## Purpose

**Short**: sibling Markdown must not supply accepted facts

## Extra Raw Section

raw-only-marker
`)},
		"rhoai.next/typed-only/.analyzer/component-architecture.json": {Data: []byte(`{"component":"typed-only","controller_watches":[{"type":"Owns","gvk":"wrong/v1.Fact","controller":"wrong","source":"wrong.go"}]}`)},
	}
	data, err := LoadVersion(fsys, nil, "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	doc := data.Components["typed-only"]
	if doc == nil || doc.Name != "typed-only" || len(doc.ControllerWatches) != 1 || doc.ControllerWatches[0].GVK != "apps/v1.Deployment" {
		t.Fatalf("accepted document was not authoritative: %#v", doc)
	}
	if strings.Contains(doc.Purpose, "sibling Markdown") {
		t.Fatalf("sibling Markdown supplied accepted facts: %q", doc.Purpose)
	}
	if !strings.Contains(doc.RawSections["Extra Raw Section"], "raw-only-marker") {
		t.Fatalf("rendered sibling raw sections unavailable: %#v", doc.RawSections)
	}
}

func TestLoadVersionInvalidAcceptedDocumentNeverFallsBack(t *testing.T) {
	otherIdentity := readFixture(t, "../documentdata/testdata/rhoai.next/praxis-policy/document.json")
	tests := map[string]struct {
		document []byte
		want     string
	}{
		"unsupported": {[]byte(`{"schema_version":"2.0.0"}`), "schema_version"},
		"identity mismatch": {
			otherIdentity,
			"publication directory",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"rhoai.next/typed-only/document.json":                         {Data: test.document},
				"rhoai.next/typed-only.md":                                    {Data: []byte("# Component: typed-only\n\n## Purpose\n\nlegacy fallback\n")},
				"rhoai.next/typed-only/.analyzer/component-architecture.json": {Data: []byte(`{"component":"typed-only"}`)},
			}
			data, err := LoadVersion(fsys, nil, "rhoai.next")
			if err == nil || data != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadVersion = %#v, %v; want actionable error containing %q", data, err, test.want)
			}
		})
	}
}

func TestLoadVersionIncompletePublicationNeverFallsBack(t *testing.T) {
	for _, artifact := range []string{"analyzer.json", ".document.json.next"} {
		t.Run(artifact, func(t *testing.T) {
			fsys := fstest.MapFS{
				"rhoai.next/typed-only.md":          {Data: []byte("# Component: typed-only\n\n## Purpose\n\nlegacy fallback\n")},
				"rhoai.next/typed-only/" + artifact: {Data: []byte(`{}`)},
			}
			data, err := LoadVersion(fsys, nil, "rhoai.next")
			if err == nil || data != nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("LoadVersion = %#v, %v; want incomplete-publication error", data, err)
			}
		})
	}
}

func TestLoadVersionJSONOnlyAcceptedAndMixedLegacyCorpus(t *testing.T) {
	document := readFixture(t, "../documentdata/testdata/rhoai.next/typed-only/document.json")
	fsys := fstest.MapFS{
		"rhoai.next/typed-only/document.json":                     {Data: document},
		"rhoai.next/typed-only/analyzer.json":                     {Data: []byte(`{"component":"typed-only"}`)},
		"rhoai.next/legacy.md":                                    {Data: []byte("# Component: legacy\n\n## Purpose\n\nlegacy component\n")},
		"rhoai.next/legacy/.analyzer/component-architecture.json": {Data: []byte(`{"component":"legacy","controller_watches":[{"type":"Owns","gvk":"v1.Pod","controller":"legacy","source":"legacy.go"}]}`)},
		"rhoai.next/run-metadata/attempts/01/response.json":       {Data: []byte(`{"component":"must-not-load"}`)},
		"rhoai.next/run-metadata/document.json":                   {Data: document},
		"rhoai.next/contracts/schemas/example/schema.json":        {Data: []byte(`{"component":"must-not-load"}`)},
	}
	data, err := LoadVersion(fsys, nil, "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	if data.Version.ComponentCount != 2 || len(data.Components) != 2 {
		t.Fatalf("mixed component count = %d/%d, want 2", data.Version.ComponentCount, len(data.Components))
	}
	accepted := data.Components["typed-only"]
	if accepted == nil || accepted.FileName != "" || accepted.RawSections != nil || len(accepted.Webhooks) != 1 {
		t.Fatalf("JSON-only accepted component unavailable for typed queries: %#v", accepted)
	}
	if legacy := data.Components["legacy"]; legacy == nil || len(legacy.ControllerWatches) != 1 {
		t.Fatalf("legacy Markdown+analyzer path changed: %#v", legacy)
	}
	versions, err := DiscoverVersions(fsys, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ComponentCount != 2 {
		t.Fatalf("mixed version inventory = %#v", versions)
	}
}

func TestLoadVersionPublishedDerivativeFreshnessAndJSONOnlyAvailability(t *testing.T) {
	complete := publishedFixture(t, true)
	data, err := LoadVersion(complete, nil, "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	if data.Components["typed-only"].FileName != "typed-only.md" {
		t.Fatalf("published Markdown availability lost: %#v", data.Components["typed-only"])
	}

	jsonOnly := publishedFixture(t, false)
	data, err = LoadVersion(jsonOnly, nil, "rhoai.next")
	if err != nil {
		t.Fatal(err)
	}
	if data.Components["typed-only"].FileName != "" {
		t.Fatalf("missing derivative reported as available: %#v", data.Components["typed-only"])
	}

	tampered := publishedFixture(t, true)
	markdown := tampered["rhoai.next/typed-only.md"]
	markdown.Data = append(append([]byte{}, markdown.Data...), []byte("tampered body\n")...)
	tampered["rhoai.next/typed-only.md"] = markdown
	if loaded, loadErr := LoadVersion(tampered, nil, "rhoai.next"); loadErr == nil || loaded != nil || !strings.Contains(loadErr.Error(), "stale or tampered") {
		t.Fatalf("tampered derivative LoadVersion = %#v, %v", loaded, loadErr)
	}
}

func publishedFixture(t *testing.T, withMarkdown bool) fstest.MapFS {
	t.Helper()
	document := map[string]any{}
	if err := json.Unmarshal(readFixture(t, "../documentdata/testdata/rhoai.next/typed-only/document.json"), &document); err != nil {
		t.Fatal(err)
	}
	identity := document["identity"].(map[string]any)
	producers := document["producers"].(map[string]any)
	analyzerInput := document["analyzer_input"].(map[string]any)
	analyzer := map[string]any{
		"schema_version": "1", "component": identity["source_component"],
		"repo": identity["repository"], "commit_sha": identity["source_revision"],
		"extracted_at":     analyzerInput["extracted_at"],
		"analyzer_version": producers["analyzer_version"],
	}
	analyzerData, err := json.Marshal(analyzer)
	if err != nil {
		t.Fatal(err)
	}
	bundleFingerprint := testHash(analyzerData)
	analyzerInput["bundle_fingerprint"] = bundleFingerprint
	hash := "sha256:" + strings.Repeat("a", 64)
	synthesis := map[string]any{
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
	synthesisData, err := json.Marshal(synthesis)
	if err != nil {
		t.Fatal(err)
	}
	document["schema_version"] = "1.1.0"
	document["publication"] = map[string]any{
		"contract": "structured-component-publication/v1", "snapshot_id": hash,
		"analyzer": map[string]any{
			"content_hash": testHash(analyzerData), "bundle_fingerprint": bundleFingerprint,
			"schema_version": "1", "source_component": identity["source_component"],
			"repository": identity["repository"], "source_revision": identity["source_revision"],
			"analyzer_version": producers["analyzer_version"], "producer_build_identity": hash,
		},
		"synthesis": map[string]any{
			"content_hash": testHash(synthesisData), "state": "deterministic-only",
			"input_bundle_identity": hash, "current_evidence_bundle_identity": hash,
			"original_evidence_bundle_identity": hash, "producing_model_eligible": false,
		},
		"accepted_inputs": map[string]any{
			"run_record":               map[string]any{"fixture": true},
			"current_evidence_bundle":  map[string]any{"fixture": true},
			"original_evidence_bundle": map[string]any{"fixture": true},
		},
		"markdown": map[string]any{"path": "typed-only.md", "renderer_version": "arch-analyzer-markdown/v1"},
		"diagram":  map[string]any{"state": "unavailable"},
	}
	documentData, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	result := fstest.MapFS{
		"rhoai.next/typed-only/document.json":  {Data: documentData},
		"rhoai.next/typed-only/analyzer.json":  {Data: analyzerData},
		"rhoai.next/typed-only/synthesis.json": {Data: synthesisData},
	}
	if withMarkdown {
		markdown, renderErr := analyzerdocument.RenderMarkdownJSON(documentData)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		result["rhoai.next/typed-only.md"] = &fstest.MapFile{Data: markdown}
	}
	return result
}

func testHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestDiscoverVersionsPreservesSortedAliases(t *testing.T) {
	fsys := fstest.MapFS{"rhoai-1/example.md": {Data: []byte("# Component: example\n")}}
	versions, err := DiscoverVersions(fsys, map[string]string{"z-alias": "rhoai-1", "a-alias": "rhoai-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || strings.Join(versions[0].Aliases, ",") != "a-alias,z-alias" {
		t.Fatalf("version aliases = %#v", versions)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLoadVersionExcludesVersionIndexFromComponents(t *testing.T) {
	fsys := fstest.MapFS{
		"rhoai.next/INDEX.md": {Data: []byte("# Index\n")},
		"rhoai.next/example.md": {Data: []byte(`# Component: example

## Overview

Example component.
`)},
	}

	data, err := LoadVersion(fsys, nil, "rhoai.next")
	if err != nil {
		t.Fatalf("LoadVersion: %v", err)
	}
	if _, exists := data.Components["INDEX"]; exists {
		t.Fatal("INDEX.md was loaded as a component")
	}
	if data.Components["example"] == nil {
		t.Fatal("example component was not loaded")
	}

	versions, err := DiscoverVersions(fsys, nil)
	if err != nil {
		t.Fatalf("DiscoverVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].ComponentCount != 1 {
		t.Fatalf("expected one component in version inventory, got %#v", versions)
	}
}
