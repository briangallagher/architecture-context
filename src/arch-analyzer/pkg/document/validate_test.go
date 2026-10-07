package document

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/structured"
)

func TestValidateJSONAcceptsPhaseOneDocument(t *testing.T) {
	raw := phaseOneDocument(t)
	if err := ValidateJSON(raw); err != nil {
		t.Fatalf("ValidateJSON() error = %v", err)
	}
}

func TestValidateJSONRejectsInvalidDocuments(t *testing.T) {
	valid := phaseOneDocument(t)
	tests := []struct {
		name   string
		mutate func(t *testing.T, value map[string]any)
		want   string
	}{
		{
			name: "unknown field",
			mutate: func(_ *testing.T, value map[string]any) {
				value["future_field"] = true
			},
			want: "unknown field",
		},
		{
			name: "wrong type",
			mutate: func(_ *testing.T, value map[string]any) {
				value["facts"] = "not-an-array"
			},
			want: "cannot unmarshal",
		},
		{
			name: "invalid structure",
			mutate: func(_ *testing.T, value map[string]any) {
				delete(value, "assembly_inputs")
			},
			want: "assembly_inputs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(valid, &value); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, value)
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateJSON(raw)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateJSON() error = %v, want substring %q", err, test.want)
			}
		})
	}

	if err := ValidateJSON([]byte(`{"schema_version":`)); err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("malformed ValidateJSON() error = %v, want unexpected EOF", err)
	}
}

func phaseOneDocument(t *testing.T) []byte {
	t.Helper()
	analyzer := `{
  "component":"example",
  "repo":"https://github.com/example/component.git",
  "commit_sha":"0123456789abcdef",
  "extracted_at":"2026-09-07T12:00:00Z",
  "analyzer_version":"phase-one-test",
  "schema_version":"1",
  "summary":"Example component.",
  "source_components":[{"name":"api","type":"service","purpose":"Serves requests","source":"cmd/api.go:1-20"}]
}`
	document, err := structured.Normalize(strings.NewReader(analyzer), structured.Options{
		Distribution:      "RHOAI",
		GeneratedBy:       "pkg/document test",
		VersionScope:      "rhoai-test",
		IntegrationStatus: "current",
	})
	if err != nil {
		t.Fatalf("phase-one Normalize() error = %v", err)
	}
	var encoded bytes.Buffer
	if err := structured.Encode(&encoded, document); err != nil {
		t.Fatalf("phase-one Encode() error = %v", err)
	}
	return encoded.Bytes()
}
