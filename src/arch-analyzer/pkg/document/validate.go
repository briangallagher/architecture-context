// Package document exposes validation for accepted structured component
// documents without exposing the analyzer's internal document representation.
package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/jctanner/arch-analyzer/internal/renderer"
	"github.com/jctanner/arch-analyzer/internal/structured"
)

// ValidateJSON strictly decodes and semantically validates an accepted
// structured component document.
func ValidateJSON(data []byte) error {
	_, err := structured.Decode(bytes.NewReader(data))
	return err
}

const markerFormat = "<!-- structured-component: document_sha256=%s renderer=%s body_sha256=%s -->\n"

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RenderMarkdownJSON uses the sole analyzer renderer and adds the publication
// marker used by consumers to bind a derivative to exact authority bytes.
func RenderMarkdownJSON(data []byte) ([]byte, error) {
	document, err := structured.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if document.Publication == nil {
		return nil, fmt.Errorf("accepted document is not a published snapshot")
	}
	body, err := RenderBodyJSON(data)
	if err != nil {
		return nil, err
	}
	marker := fmt.Sprintf(markerFormat, hash(data), document.Producers.RendererVersion, hash(body))
	return append([]byte(marker), body...), nil
}

// RenderBodyJSON exposes the producer-owned compatibility projection for tests
// and consumers that must compare against the current renderer implementation.
func RenderBodyJSON(data []byte) ([]byte, error) {
	document, err := structured.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	if err := renderer.AcceptedMarkdown(&body, document); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

// ValidatePublicationJSON validates exact sibling bytes and the Markdown body.
// Hashes establish artifact identity; structured validation remains responsible
// for evidence and semantic integrity.
func ValidateAuthorityJSON(documentData, analyzerData, synthesisData []byte) error {
	document, err := structured.Decode(bytes.NewReader(documentData))
	if err != nil {
		return err
	}
	if document.Publication == nil {
		return fmt.Errorf("accepted document is not a published snapshot")
	}
	if got := hash(analyzerData); got != document.Publication.Analyzer.ContentHash {
		return fmt.Errorf("published analyzer hash mismatch: got %s", got)
	}
	if got := hash(synthesisData); got != document.Publication.Synthesis.ContentHash {
		return fmt.Errorf("published synthesis hash mismatch: got %s", got)
	}
	var analyzer struct {
		SchemaVersion   string `json:"schema_version"`
		Component       string `json:"component"`
		Repo            string `json:"repo"`
		CommitSHA       string `json:"commit_sha"`
		ExtractedAt     string `json:"extracted_at"`
		AnalyzerVersion string `json:"analyzer_version"`
	}
	if err := strictJSON(analyzerData, &analyzer); err != nil {
		return fmt.Errorf("decode published analyzer: %w", err)
	}
	bound := document.Publication.Analyzer
	if analyzer.SchemaVersion != bound.SchemaVersion || analyzer.Component != bound.SourceComponent || analyzer.Repo != bound.Repository || analyzer.CommitSHA != bound.SourceRevision || analyzer.AnalyzerVersion != bound.AnalyzerVersion {
		return fmt.Errorf("published analyzer identity does not match document binding")
	}
	if analyzer.ExtractedAt != document.AnalyzerInput.ExtractedAt {
		return fmt.Errorf("published analyzer extraction time does not match document binding")
	}
	var analyzerValue any
	if err := strictJSON(analyzerData, &analyzerValue); err != nil {
		return fmt.Errorf("decode published analyzer value: %w", err)
	}
	canonicalAnalyzer, err := json.Marshal(analyzerValue)
	if err != nil {
		return fmt.Errorf("canonicalize published analyzer: %w", err)
	}
	if hash(canonicalAnalyzer) != bound.BundleFingerprint {
		return fmt.Errorf("published analyzer bundle fingerprint mismatch")
	}
	var synthesis struct {
		State                    string  `json:"state"`
		InputBundleIdentity      string  `json:"input_bundle_identity"`
		AcceptedResponseIdentity *string `json:"accepted_response_identity"`
	}
	if err := strictJSON(synthesisData, &synthesis); err != nil {
		return fmt.Errorf("decode published synthesis: %w", err)
	}
	synthesisBound := document.Publication.Synthesis
	if synthesis.State != synthesisBound.State || synthesis.InputBundleIdentity != synthesisBound.InputBundleIdentity {
		return fmt.Errorf("published synthesis identity does not match document binding")
	}
	accepted := ""
	if synthesis.AcceptedResponseIdentity != nil {
		accepted = *synthesis.AcceptedResponseIdentity
	}
	if accepted != synthesisBound.AcceptedResponseIdentity {
		return fmt.Errorf("published synthesis accepted response does not match document binding")
	}
	if synthesis.State == "synthesized" {
		if err := validateDurableAcceptance(document, analyzer, synthesis); err != nil {
			return err
		}
	}
	return nil
}

func validateDurableAcceptance(document structured.Document, analyzer struct {
	SchemaVersion   string `json:"schema_version"`
	Component       string `json:"component"`
	Repo            string `json:"repo"`
	CommitSHA       string `json:"commit_sha"`
	ExtractedAt     string `json:"extracted_at"`
	AnalyzerVersion string `json:"analyzer_version"`
}, synthesis struct {
	State                    string  `json:"state"`
	InputBundleIdentity      string  `json:"input_bundle_identity"`
	AcceptedResponseIdentity *string `json:"accepted_response_identity"`
}) error {
	var record struct {
		Component string `json:"component"`
		Identity  struct {
			Component string   `json:"component"`
			Aliases   []string `json:"aliases"`
		} `json:"identity"`
		Inputs struct {
			AnalyzerBinding struct {
				Component         string `json:"component"`
				Repository        string `json:"repository"`
				CommitSHA         string `json:"commit_sha"`
				ExtractedAt       string `json:"extracted_at"`
				AnalyzerVersion   string `json:"analyzer_version"`
				SchemaVersion     string `json:"schema_version"`
				BundleFingerprint string `json:"bundle_fingerprint"`
			} `json:"analyzer_binding"`
		} `json:"inputs"`
		Compatibility struct {
			AnalyzerBuildIdentity string `json:"analyzer_build_identity"`
		} `json:"compatibility"`
		Dependencies struct {
			Complete bool `json:"complete"`
		} `json:"dependencies"`
		SourceState struct {
			End struct {
				Head string `json:"head"`
			} `json:"end"`
		} `json:"source_state"`
		ResponseIdentity  string `json:"response_identity"`
		DocumentIntegrity string `json:"document_integrity"`
	}
	if err := strictJSON(document.Publication.AcceptedInputs.RunRecord, &record); err != nil {
		return fmt.Errorf("decode published acceptance record: %w", err)
	}
	if _, err := objectIdentity(document.Publication.AcceptedInputs.RunRecord, "record_identity"); err != nil {
		return fmt.Errorf("validate published acceptance record identity: %w", err)
	}
	if record.Component != document.Identity.Component || record.Identity.Component != document.Identity.Component || !reflect.DeepEqual(record.Identity.Aliases, document.Identity.Aliases) {
		return fmt.Errorf("published aliases or component differ from acceptance record")
	}
	binding := record.Inputs.AnalyzerBinding
	if binding.Component != analyzer.Component || binding.Repository != analyzer.Repo || binding.CommitSHA != analyzer.CommitSHA || binding.ExtractedAt != analyzer.ExtractedAt || binding.AnalyzerVersion != analyzer.AnalyzerVersion || binding.SchemaVersion != analyzer.SchemaVersion || binding.BundleFingerprint != document.Publication.Analyzer.BundleFingerprint {
		return fmt.Errorf("published analyzer differs from parent-recorded accepted input")
	}
	if record.Compatibility.AnalyzerBuildIdentity != document.Publication.Analyzer.ProducerBuildIdentity {
		return fmt.Errorf("published analyzer build differs from acceptance record")
	}
	if !record.Dependencies.Complete || record.SourceState.End.Head != analyzer.CommitSHA {
		return fmt.Errorf("published dependency or source provenance is incomplete")
	}
	if synthesis.AcceptedResponseIdentity == nil || record.ResponseIdentity != *synthesis.AcceptedResponseIdentity {
		return fmt.Errorf("published response differs from acceptance record")
	}
	privateHash, err := privateDocumentHash(document)
	if err != nil {
		return err
	}
	if record.DocumentIntegrity != privateHash {
		return fmt.Errorf("published document does not reconstruct the accepted model")
	}
	if err := validateBundleBindings(document, synthesis.InputBundleIdentity); err != nil {
		return err
	}
	if err := validateEvidenceRevisions(document, analyzer.CommitSHA); err != nil {
		return err
	}
	return nil
}

func ValidatePublicationJSON(documentData, analyzerData, synthesisData, markdownData []byte) error {
	if err := ValidateAuthorityJSON(documentData, analyzerData, synthesisData); err != nil {
		return err
	}
	expected, err := RenderMarkdownJSON(documentData)
	if err != nil {
		return fmt.Errorf("render published Markdown: %w", err)
	}
	if !bytes.Equal(expected, markdownData) {
		return fmt.Errorf("published Markdown is stale or tampered")
	}
	return nil
}

func privateDocumentHash(document structured.Document) (string, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode published document: %w", err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode published document: %w", err)
	}
	delete(value, "publication")
	value["schema_version"] = structured.DocumentSchemaVersion
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", fmt.Errorf("canonicalize private document: %w", err)
	}
	return hash(canonical), nil
}

func validateBundleBindings(document structured.Document, synthesisInput string) error {
	type bundleHeader struct {
		BundleIdentity string `json:"bundle_identity"`
		Analyzer       struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"analyzer"`
	}
	var current, original bundleHeader
	if err := strictJSON(document.Publication.AcceptedInputs.CurrentEvidenceBundle, &current); err != nil {
		return fmt.Errorf("decode current evidence bundle: %w", err)
	}
	if err := strictJSON(document.Publication.AcceptedInputs.OriginalEvidenceBundle, &original); err != nil {
		return fmt.Errorf("decode original evidence bundle: %w", err)
	}
	bound := document.Publication.Synthesis
	if current.BundleIdentity != bound.CurrentEvidenceBundleIdentity ||
		original.BundleIdentity != bound.OriginalEvidenceBundleIdentity ||
		original.BundleIdentity != synthesisInput {
		return fmt.Errorf("published evidence bundle identity mismatch")
	}
	for label, raw := range map[string]json.RawMessage{
		"current":  document.Publication.AcceptedInputs.CurrentEvidenceBundle,
		"original": document.Publication.AcceptedInputs.OriginalEvidenceBundle,
	} {
		identity, err := objectIdentity(raw, "bundle_identity")
		if err != nil {
			return fmt.Errorf("validate %s evidence bundle identity: %w", label, err)
		}
		if (label == "current" && identity != current.BundleIdentity) ||
			(label == "original" && identity != original.BundleIdentity) {
			return fmt.Errorf("published %s evidence bundle identity is invalid", label)
		}
	}
	var analyzerValue any
	decoder := json.NewDecoder(bytes.NewReader(current.Analyzer.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&analyzerValue); err != nil {
		return fmt.Errorf("decode current evidence analyzer payload: %w", err)
	}
	canonicalAnalyzer, err := json.Marshal(analyzerValue)
	if err != nil {
		return fmt.Errorf("canonicalize current evidence analyzer payload: %w", err)
	}
	if hash(canonicalAnalyzer) != document.Publication.Analyzer.BundleFingerprint {
		return fmt.Errorf("current evidence analyzer differs from published analyzer")
	}
	return nil
}

func objectIdentity(raw []byte, identityField string) (string, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	claimed, ok := value[identityField].(string)
	if !ok {
		return "", fmt.Errorf("missing %s", identityField)
	}
	delete(value, identityField)
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	if claimed != hash(canonical) {
		return "", fmt.Errorf("%s hash mismatch", identityField)
	}
	return claimed, nil
}

func validateEvidenceRevisions(document structured.Document, revision string) error {
	for _, fact := range document.Facts {
		for _, evidence := range fact.Evidence {
			if evidence.Revision != revision {
				return fmt.Errorf("published fact evidence revision differs from analyzer source")
			}
		}
	}
	for _, disposition := range document.Dispositions {
		for _, evidence := range disposition.Evidence {
			if evidence.Revision != revision {
				return fmt.Errorf("published disposition evidence revision differs from analyzer source")
			}
		}
	}
	for _, section := range document.Sections {
		for _, evidence := range section.Evidence {
			if evidence.Revision != revision {
				return fmt.Errorf("published section evidence revision differs from analyzer source")
			}
		}
		for _, block := range section.Blocks {
			for _, row := range block.Rows {
				for _, evidence := range row.Evidence {
					if evidence.Revision != revision {
						return fmt.Errorf("published table evidence revision differs from analyzer source")
					}
				}
			}
		}
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte("\n")), nil
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
