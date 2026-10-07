package structured

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/model"
)

func TestFactRegistryCoversEveryAnalyzerFactField(t *testing.T) {
	topFields := map[string]bool{}
	typeOfInput := reflect.TypeOf(model.Input{})
	for index := 0; index < typeOfInput.NumField(); index++ {
		field := typeOfInput.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" && !metadataFields[name] {
			topFields[name] = true
		}
	}
	covered := map[string]bool{}
	for _, descriptor := range factRegistry {
		covered[descriptor.Top] = true
	}
	if !reflect.DeepEqual(topFields, covered) {
		t.Fatalf("fact registry/top-level Input mismatch\ninput: %#v\nregistry: %#v", topFields, covered)
	}

	assertNestedCoverage(t, reflect.TypeOf(model.RBAC{}), "rbac")
	assertNestedCoverage(t, reflect.TypeOf(model.Dependencies{}), "dependencies")
}

func TestScanStatisticsAreAuditMetadataNotAcceptedFacts(t *testing.T) {
	base := `{
      "component":"example","repo":"org/example","commit_sha":"abc",
      "extracted_at":"now","analyzer_version":"test","schema_version":"1",
      "scan_statistics":[{"category":"authentication","metric":"files_scanned","value":12,"unit":"files","scope":"Python source"}]
    }`
	document, err := Normalize(strings.NewReader(base), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range document.Facts {
		if fact.Type == "scan_statistic" || strings.Contains(fact.InputPointer, "scan_statistics") {
			t.Fatalf("scan statistic became an accepted fact: %#v", fact)
		}
	}

	changed := strings.Replace(base, `"value":12`, `"value":13`, 1)
	changedDocument, err := Normalize(strings.NewReader(changed), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if document.AnalyzerInput.BundleFingerprint == changedDocument.AnalyzerInput.BundleFingerprint {
		t.Fatal("exact analyzer audit fingerprint ignored changed scan statistics")
	}
}

func assertNestedCoverage(t *testing.T, object reflect.Type, top string) {
	t.Helper()
	want := map[string]bool{}
	for index := 0; index < object.NumField(); index++ {
		name := strings.Split(object.Field(index).Tag.Get("json"), ",")[0]
		want[name] = true
	}
	got := map[string]bool{}
	for _, descriptor := range factRegistry {
		if descriptor.Top == top {
			got[descriptor.Child] = true
		}
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("%s nested registry mismatch\nwant: %#v\ngot: %#v", top, want, got)
	}
}

func TestNormalizeAccountsForEveryRegisteredFactType(t *testing.T) {
	top := map[string]any{
		"component": "inventory", "repo": "example/inventory", "commit_sha": "abc123",
		"extracted_at": "2026-09-06T12:00:00Z", "analyzer_version": "test", "schema_version": "1",
	}
	nested := map[string]map[string]any{}
	for _, descriptor := range factRegistry {
		var value any
		switch descriptor.Type {
		case "summary", "dependency_go_version", "data_coverage":
			value = "value"
		case "context_contract":
			value = model.ContextContract{ContractVersion: "1"}
		default:
			encoded, err := json.Marshal(descriptor.NewValue())
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
		}
		switch descriptor.Shape {
		case shapeSingleton:
			top[descriptor.Top] = value
		case shapeArray:
			top[descriptor.Top] = []any{value}
		case shapeNestedSingleton:
			if nested[descriptor.Top] == nil {
				nested[descriptor.Top] = map[string]any{}
			}
			nested[descriptor.Top][descriptor.Child] = value
		case shapeNestedArray:
			if nested[descriptor.Top] == nil {
				nested[descriptor.Top] = map[string]any{}
			}
			nested[descriptor.Top][descriptor.Child] = []any{value}
		case shapeMap:
			top[descriptor.Top] = map[string]any{"inventory": value}
		case shapeMapArray:
			top[descriptor.Top] = map[string]any{"inventory": []any{value}}
		}
	}
	for key, value := range nested {
		top[key] = value
	}
	raw, err := json.Marshal(top)
	if err != nil {
		t.Fatal(err)
	}
	document, err := Normalize(bytes.NewReader(raw), Options{})
	if err != nil {
		t.Fatal(err)
	}
	gotTypes := make([]string, 0, len(document.Facts))
	for _, fact := range document.Facts {
		gotTypes = append(gotTypes, fact.Type)
	}
	sort.Strings(gotTypes)
	wantTypes := FactTypes()
	sort.Strings(wantTypes)
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("accounted fact types mismatch\n got: %v\nwant: %v", gotTypes, wantTypes)
	}
	if len(document.FactAccounting) != len(document.Facts) {
		t.Fatalf("accounting=%d facts=%d", len(document.FactAccounting), len(document.Facts))
	}
}

func TestStableFactIDsIgnoreObjectFormattingAndArrayOrder(t *testing.T) {
	left := `{
      "component":"example","repo":"org/example","commit_sha":"abc","extracted_at":"now","analyzer_version":"test","schema_version":"1",
      "services":[{"name":"b","source":"b.yaml","type":"ClusterIP","ports":[],"target_deployment":""},{"name":"a","source":"a.yaml","type":"ClusterIP","ports":[],"target_deployment":""}]
    }`
	right := `{"schema_version":"1","analyzer_version":"test","extracted_at":"now","commit_sha":"abc","repo":"org/example","component":"example","services":[{"target_deployment":"","ports":[],"type":"ClusterIP","source":"a.yaml","name":"a"},{"target_deployment":"","ports":[],"type":"ClusterIP","source":"b.yaml","name":"b"}]}`
	leftDocument, err := Normalize(strings.NewReader(left), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rightDocument, err := Normalize(strings.NewReader(right), Options{})
	if err != nil {
		t.Fatal(err)
	}
	leftIDs, rightIDs := []string{}, []string{}
	for _, fact := range leftDocument.Facts {
		leftIDs = append(leftIDs, fact.ID)
	}
	for _, fact := range rightDocument.Facts {
		rightIDs = append(rightIDs, fact.ID)
	}
	sort.Strings(leftIDs)
	sort.Strings(rightIDs)
	if !reflect.DeepEqual(leftIDs, rightIDs) {
		t.Fatalf("stable IDs changed with array/object order\nleft=%v\nright=%v", leftIDs, rightIDs)
	}
	if leftDocument.AnalyzerInput.BundleFingerprint == rightDocument.AnalyzerInput.BundleFingerprint {
		t.Fatal("bundle fingerprint should retain meaningful array ordering")
	}
}

func TestAnalyzerBundleFingerprintUsesCanonicalNestedJSONBytes(t *testing.T) {
	left := `{"component":"example","repo":"org/example","commit_sha":"abc","extracted_at":"now","analyzer_version":"test","schema_version":"1","services":[{"name":"api","source":"service.yaml","type":"ClusterIP","ports":[],"target_deployment":"api"}]}`
	right := `{
	  "services": [{"target_deployment":"api", "ports": [], "type":"ClusterIP", "source":"service.yaml", "name":"api"}],
	  "schema_version":"1", "analyzer_version":"test", "extracted_at":"now", "commit_sha":"abc", "repo":"org/example", "component":"example"
	}`
	leftDocument, err := Normalize(strings.NewReader(left), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rightDocument, err := Normalize(strings.NewReader(right), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if leftDocument.AnalyzerInput.BundleFingerprint != rightDocument.AnalyzerInput.BundleFingerprint {
		t.Fatalf("canonical bundle fingerprints differ: %s != %s", leftDocument.AnalyzerInput.BundleFingerprint, rightDocument.AnalyzerInput.BundleFingerprint)
	}
}

func TestComponentMapInputIsRetainedAndRepoLineageIsRecomputed(t *testing.T) {
	componentMap := &model.ComponentMap{
		Components: map[string]model.ComponentEntry{
			"example-canonical": {RepoOrg: "downstream", RepoName: "example", RepoURL: "https://github.com/downstream/example"},
		},
		Provenance: &model.ComponentMapProvenance{Repos: map[string]model.ComponentMapRepo{
			"downstream/example": {
				Org: "downstream", Repo: "example", IsFork: true, Upstream: "upstream/example",
				UpstreamDetection: "github-parent", SyncMechanism: "merge", SyncBranch: "main",
			},
			"upstream/example": {Org: "upstream", Repo: "example"},
		}},
	}
	analyzer := `{"component":"example","repo":"https://github.com/downstream/example.git","commit_sha":"abc","schema_version":"1","summary":"example"}`
	document, err := Normalize(strings.NewReader(analyzer), Options{ComponentMap: componentMap, ComponentMapID: "release-component-map"})
	if err != nil {
		t.Fatal(err)
	}
	if document.AssemblyInputs.ComponentMap == nil || document.AssemblyInputs.ComponentMap.Origin.ID != "release-component-map" {
		t.Fatalf("component-map input attribution missing: %#v", document.AssemblyInputs.ComponentMap)
	}
	if document.Identity.Component != "example-canonical" || len(document.RenderingView.RepoLineage) != 2 {
		t.Fatalf("component-map projection missing: identity=%#v lineage=%#v", document.Identity, document.RenderingView.RepoLineage)
	}

	tamperedView := document
	tamperedView.RenderingView.RepoLineage = append([]model.RepoLineageRow{}, document.RenderingView.RepoLineage...)
	tamperedView.RenderingView.RepoLineage[0].Repository = "https://example.invalid/spoofed"
	if err := Validate(tamperedView); err == nil || !strings.Contains(err.Error(), "parent mapping inputs") {
		t.Fatalf("tampered repo lineage Validate() error=%v", err)
	}

	tamperedInput := document
	copyInput := *document.AssemblyInputs.ComponentMap
	tamperedInput.AssemblyInputs.ComponentMap = &copyInput
	copyInput.Value.Components = map[string]model.ComponentEntry{"spoofed": {RepoOrg: "bad", RepoName: "bad"}}
	if err := Validate(tamperedInput); err == nil || !strings.Contains(err.Error(), "content_fingerprint mismatch") {
		t.Fatalf("tampered component-map input Validate() error=%v", err)
	}
}

func TestEmptyComponentMapProvenanceNormalizesToSchemaProjection(t *testing.T) {
	componentMap := &model.ComponentMap{
		Components: map[string]model.ComponentEntry{},
		Provenance: &model.ComponentMapProvenance{},
	}
	document, err := Normalize(strings.NewReader(`{"component":"example","schema_version":"1"}`), Options{ComponentMap: componentMap})
	if err != nil {
		t.Fatal(err)
	}
	provenance := document.AssemblyInputs.ComponentMap.Value.Provenance
	if provenance == nil || provenance.Repos == nil || len(provenance.Repos) != 0 {
		t.Fatalf("normalized component-map provenance = %#v, want explicit empty repos object", provenance)
	}
	var encoded bytes.Buffer
	if err := Encode(&encoded, document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded.String(), `"provenance": {`) || !strings.Contains(encoded.String(), `"repos": {}`) {
		t.Fatalf("encoded component-map provenance is not schema-shaped:\n%s", encoded.String())
	}
}

func TestValidateBindsIntegrationUncertaintyToIdentity(t *testing.T) {
	document, err := Normalize(strings.NewReader(`{"component":"example","schema_version":"1"}`), Options{IntegrationStatus: "not-integrated"})
	if err != nil {
		t.Fatal(err)
	}
	document.Identity.IntegrationStatus = "current"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "does not match identity integration_status") {
		t.Fatalf("tampered integration identity Validate() error=%v", err)
	}
}

func TestValidateRejectsControlCharactersInFactEvidencePathAndRevision(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*EvidenceRef)
	}{
		{"path", func(reference *EvidenceRef) { reference.Path = "config/service.yaml\n```" }},
		{"revision", func(reference *EvidenceRef) { reference.Revision = "abc\n```" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Normalize(strings.NewReader(`{"component":"example","commit_sha":"abc","schema_version":"1","services":[{"name":"api","source":"config/service.yaml","type":"ClusterIP","ports":[],"target_deployment":"api"}]}`), Options{})
			if err != nil {
				t.Fatal(err)
			}
			fact := factOfType(t, document, "service")
			for index := range document.Facts {
				if document.Facts[index].ID == fact.ID {
					test.mutate(&document.Facts[index].Evidence[0])
				}
			}
			if err := Validate(document); err == nil || !strings.Contains(err.Error(), "single line without control") {
				t.Fatalf("Validate() error=%v", err)
			}
		})
	}
}

func TestNormalizeRejectsUnsupportedAndInvalidAnalyzerStructures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"unsupported top-level fact", `{"component":"x","schema_version":"1","future_required_facts":[]}`, "unsupported analyzer fact"},
		{"unsupported schema", `{"component":"x","schema_version":"2"}`, "unsupported analyzer schema_version"},
		{"unknown nested RBAC", `{"component":"x","schema_version":"1","rbac":{"future_rules":[]}}`, "unknown field"},
		{"wrong typed fact", `{"component":"x","schema_version":"1","services":["not-an-object"]}`, "cannot unmarshal string"},
		{"malformed source range", `{"component":"x","schema_version":"1","services":[{"name":"x","source":"service.yaml:10-2","type":"ClusterIP","ports":[],"target_deployment":"x"}]}`, "malformed source range"},
		{"absolute source path", `{"component":"x","schema_version":"1","services":[{"name":"x","source":"/tmp/service.yaml:1","type":"ClusterIP","ports":[],"target_deployment":"x"}]}`, "invalid repository-relative path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Normalize(strings.NewReader(test.raw), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsUnaccountedAndUnsupportedFacts(t *testing.T) {
	document, err := Normalize(strings.NewReader(`{"component":"x","schema_version":"1","summary":"x"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.FactAccounting = []FactAccounting{}
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "accounting incomplete") {
		t.Fatalf("Validate() error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1","summary":"x"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.Facts[0].Type = "future_required_fact"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "unsupported required fact type") {
		t.Fatalf("Validate() error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1","summary":"x"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.Facts[0].Value = json.RawMessage("null")
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "expected string value") {
		t.Fatalf("Validate() null fact error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.Sections = nil
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "explicit array values") {
		t.Fatalf("Validate() null sections error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.AssemblyInputs = nil
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "explicit assembly_inputs") {
		t.Fatalf("Validate() missing assembly inputs error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1"}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.PatchInputs = nil
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "explicit array values") {
		t.Fatalf("Validate() missing patch inputs error=%v", err)
	}

	document, err = Normalize(strings.NewReader(`{"component":"x","schema_version":"1","services":[{"name":"api","source":"service.yaml","type":"ClusterIP","ports":[],"target_deployment":"api"}]}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	document.RenderingView.Services[0].Type = "tampered"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "does not match accepted") {
		t.Fatalf("Validate() tampered rendering view error=%v", err)
	}
}
