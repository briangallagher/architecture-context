package structured

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestCentralStructuredSchemasAreValidJSONAndFactEnumMatchesRegistry(t *testing.T) {
	documentRaw, err := os.ReadFile("../../../../schemas/structured-component-document-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var documentSchema map[string]any
	if err := json.Unmarshal(documentRaw, &documentSchema); err != nil {
		t.Fatalf("document schema is not valid JSON: %v", err)
	}
	definitions := documentSchema["$defs"].(map[string]any)
	factType := definitions["factType"].(map[string]any)
	rawEnum := factType["enum"].([]any)
	got := make([]string, 0, len(rawEnum))
	for _, value := range rawEnum {
		got = append(got, value.(string))
	}
	want := FactTypes()
	if len(want) != 51 {
		t.Fatalf("registered fact type count=%d, want 51", len(want))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema fact enum differs from registry\n got: %v\nwant: %v", got, want)
	}

	patchRaw, err := os.ReadFile("../../../../schemas/structured-component-patch-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var patchSchema map[string]any
	if err := json.Unmarshal(patchRaw, &patchSchema); err != nil {
		t.Fatalf("patch schema is not valid JSON: %v", err)
	}
	patchProperties := patchSchema["properties"].(map[string]any)
	for _, forbidden := range []string{"origin", "authority", "applicability"} {
		if _, exists := patchProperties[forbidden]; exists {
			t.Fatalf("untrusted patch schema exposes trusted field %q", forbidden)
		}
	}

	policyRaw, err := os.ReadFile("../../../../schemas/structured-component-assembly-policy-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var policySchema map[string]any
	if err := json.Unmarshal(policyRaw, &policySchema); err != nil {
		t.Fatalf("assembly policy schema is not valid JSON: %v", err)
	}

	blocks := definitions["block"].(map[string]any)["oneOf"].([]any)
	var tableIDs []string
	for _, rawBlock := range blocks {
		block := rawBlock.(map[string]any)
		properties := block["properties"].(map[string]any)
		tableID, ok := properties["table_id"].(map[string]any)
		if !ok {
			continue
		}
		for _, value := range tableID["enum"].([]any) {
			tableIDs = append(tableIDs, value.(string))
		}
	}
	wantTableIDs := make([]string, 0, len(tableRegistry))
	for _, definition := range tableRegistry {
		wantTableIDs = append(wantTableIDs, definition.ID)
	}
	sort.Strings(tableIDs)
	sort.Strings(wantTableIDs)
	if !reflect.DeepEqual(tableIDs, wantTableIDs) {
		t.Fatalf("schema table enum differs from registry\n got: %v\nwant: %v", tableIDs, wantTableIDs)
	}
}
