package cmd

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jctanner/arch-query/internal/types"
)

func TestAcceptedJSONOnlyComponentRawAccessIsActionable(t *testing.T) {
	raw, err := os.ReadFile("../internal/documentdata/testdata/rhoai.next/typed-only/document.json")
	if err != nil {
		t.Fatal(err)
	}
	previousFS, previousOverlay, previousVersion, previousOutput := archFS, overlayFS, versionArg, outputFormat
	t.Cleanup(func() {
		archFS, overlayFS, versionArg, outputFormat = previousFS, previousOverlay, previousVersion, previousOutput
	})
	archFS = fstest.MapFS{"rhoai.next/typed-only/document.json": {Data: raw}}
	overlayFS = nil
	versionArg = "rhoai.next"
	outputFormat = OutputRaw

	err = componentCmd.RunE(componentCmd, []string{"typed-only"})
	if err == nil || !strings.Contains(err.Error(), "rendered Markdown is unavailable") {
		t.Fatalf("raw access error = %v", err)
	}
}

func TestExistsJSONOnlyComponentReportsAuthorityNotMissingMarkdown(t *testing.T) {
	location := formatExistsLocation(&types.ComponentDoc{DeployType: "service"}, "rhoai.next", "typed-only")
	if strings.Contains(location, "typed-only.md") || !strings.Contains(location, "typed-only/document.json") || !strings.Contains(location, "Markdown: unavailable") {
		t.Fatalf("JSON-only exists location = %q", location)
	}
}
