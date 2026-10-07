package structured

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jctanner/arch-analyzer/internal/model"
	"github.com/jctanner/arch-analyzer/internal/normalize"
)

func Normalize(reader io.Reader, options Options) (Document, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return Document{}, fmt.Errorf("read analyzer JSON: %w", err)
	}
	canonical, top, err := decodeAnalyzer(raw)
	if err != nil {
		return Document{}, err
	}

	var input model.Input
	if err := strictDecode(canonical, &input); err != nil {
		return Document{}, fmt.Errorf("decode analyzer JSON: %w", err)
	}
	if input.Component == "" {
		return Document{}, errors.New("decode analyzer JSON: missing component")
	}
	if input.SchemaVersion != "1" {
		return Document{}, fmt.Errorf("unsupported analyzer schema_version %q (supported: %q)", input.SchemaVersion, "1")
	}
	if input.ContextContract != nil {
		if err := input.ContextContract.Validate(); err != nil {
			return Document{}, fmt.Errorf("invalid context_contract: %w", err)
		}
	}
	if options.VersionScope == "" {
		options.VersionScope = "unknown"
	}
	if strings.TrimSpace(options.VersionScope) != options.VersionScope {
		return Document{}, fmt.Errorf("version_scope must not have surrounding whitespace")
	}
	if options.IntegrationStatus == "" {
		options.IntegrationStatus = "unknown"
	}

	fingerprint := digest(canonical)
	facts, err := decompose(top, input.CommitSHA)
	if err != nil {
		return Document{}, err
	}
	patchInputs, err := patchInputsFor(options.Patches)
	if err != nil {
		return Document{}, err
	}
	facts, dispositions, err := applyPatches(facts, options.Patches, options.AssemblyPolicies, fingerprint, options.VersionScope)
	if err != nil {
		return Document{}, err
	}

	assembledInput, err := compose(input, implementationFacts(facts))
	if err != nil {
		return Document{}, err
	}
	assemblyInputs, err := assemblyInputsFor(options.ComponentMap, options.ComponentMapID)
	if err != nil {
		return Document{}, err
	}
	var boundComponentMap *model.ComponentMap
	if assemblyInputs.ComponentMap != nil {
		boundComponentMap = &assemblyInputs.ComponentMap.Value
	}
	legacy := normalize.Input(assembledInput, normalize.Options{
		Distribution: options.Distribution,
		GeneratedBy:  options.GeneratedBy,
		ComponentMap: boundComponentMap,
	})
	if options.Distribution == "" {
		options.Distribution = "Both"
	}
	if options.GeneratedBy == "" {
		options.GeneratedBy = "arch-analyzer"
	}
	if facts == nil {
		facts = []Fact{}
	}
	if dispositions == nil {
		dispositions = []ProposalDisposition{}
	}
	if err := validateIntegrationStatus(options.IntegrationStatus); err != nil {
		return Document{}, err
	}
	sections := append(make([]model.StructuredSection, 0, len(options.Sections)), options.Sections...)
	for index := range sections {
		if sections[index].Blocks == nil {
			sections[index].Blocks = []model.StructuredContentBlock{}
		}
	}
	if err := ValidateSections(sections); err != nil {
		return Document{}, err
	}
	aliases := normalizedStrings(options.Aliases)
	if legacy.Component != input.Component {
		aliases = normalizedStrings(append(aliases, input.Component))
	}

	document := Document{
		SchemaVersion: DocumentSchemaVersion,
		Identity: Identity{
			Component: legacy.Component, SourceComponent: input.Component,
			Repository: input.Repo, SourceRevision: input.CommitSHA,
			VersionScope: options.VersionScope, IntegrationStatus: options.IntegrationStatus,
			Aliases: aliases,
		},
		Producers: Producers{
			AnalyzerVersion: input.AnalyzerVersion, NormalizerVersion: NormalizerVersion,
			RendererVersion: RendererVersion,
		},
		AnalyzerInput: AnalyzerInput{
			SchemaVersion: input.SchemaVersion, BundleFingerprint: fingerprint,
			ExtractedAt: input.ExtractedAt,
		},
		RenderingConfig: RenderingConfig{Distribution: options.Distribution, GeneratedBy: options.GeneratedBy},
		AssemblyInputs:  &assemblyInputs,
		Facts:           facts,
		FactAccounting:  accountingFor(facts),
		Sections:        sections,
		PatchInputs:     patchInputs,
		Dispositions:    dispositions,
		Reuse:           options.Reuse,
		Uncertainty: []Uncertainty{{
			Scope: "component-integration", Status: options.IntegrationStatus,
			Detail: "Integration status is supplied by release/component context; analyzer source facts alone do not establish shipment or roadmap intent.",
		}},
		RenderingView: legacy,
	}
	if err := Validate(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func implementationFacts(facts []Fact) []Fact {
	result := make([]Fact, 0, len(facts))
	for _, fact := range facts {
		if fact.Authority.ClaimClass == "planned" || fact.Authority.ClaimClass == "support" {
			continue
		}
		result = append(result, fact)
	}
	return result
}

func Encode(writer io.Writer, document Document) error {
	if err := Validate(document); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode structured component document: %w", err)
	}
	return nil
}

func Decode(reader io.Reader) (Document, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return Document{}, fmt.Errorf("read structured component document: %w", err)
	}
	var document Document
	if err := strictDecode(raw, &document); err != nil {
		return Document{}, fmt.Errorf("decode structured component document: %w", err)
	}
	if err := Validate(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func DecodePatch(reader io.Reader) (PatchSet, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return PatchSet{}, fmt.Errorf("read structured patch: %w", err)
	}
	var envelope struct {
		SchemaVersion json.RawMessage `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return PatchSet{}, fmt.Errorf("decode structured patch: %w", err)
	}
	if len(envelope.SchemaVersion) == 0 {
		return PatchSet{}, errors.New("decode structured patch: missing schema_version")
	}
	var version string
	if err := json.Unmarshal(envelope.SchemaVersion, &version); err != nil {
		return PatchSet{}, errors.New("ambiguous legacy patch schema_version; architecture table patch v1 is not convertible to typed fact operations")
	}
	var patch PatchSet
	if err := strictDecode(raw, &patch); err != nil {
		return PatchSet{}, fmt.Errorf("decode structured patch: %w", err)
	}
	if patch.SchemaVersion != PatchSchemaVersion {
		return PatchSet{}, fmt.Errorf("unsupported structured patch schema_version %q", patch.SchemaVersion)
	}
	return patch, nil
}

func DecodeAssemblyPolicy(reader io.Reader) (AssemblyPolicy, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return AssemblyPolicy{}, fmt.Errorf("read structured assembly policy: %w", err)
	}
	var policy AssemblyPolicy
	if err := strictDecode(raw, &policy); err != nil {
		return AssemblyPolicy{}, fmt.Errorf("decode structured assembly policy: %w", err)
	}
	if policy.SchemaVersion != PolicySchemaVersion {
		return AssemblyPolicy{}, fmt.Errorf("unsupported structured assembly policy schema_version %q", policy.SchemaVersion)
	}
	return policy, nil
}

// ProposalFingerprint returns the canonical identity used by trusted assembly
// policies. It is deliberately derived from proposal content, not a field the
// proposal author can provide.
func ProposalFingerprint(patch PatchSet) (string, error) {
	raw, err := json.Marshal(patch)
	if err != nil {
		return "", fmt.Errorf("encode patch proposal: %w", err)
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return "", fmt.Errorf("canonicalize patch proposal: %w", err)
	}
	return digest(canonical), nil
}

func patchInputsFor(patches []PatchSet) ([]PatchInputRecord, error) {
	result := make([]PatchInputRecord, 0, len(patches))
	for _, patch := range patches {
		fingerprint, err := ProposalFingerprint(patch)
		if err != nil {
			return nil, fmt.Errorf("patch %q: %w", patch.PatchID, err)
		}
		operationIDs := make([]string, 0, len(patch.Operations))
		for _, operation := range patch.Operations {
			operationIDs = append(operationIDs, operation.OperationID)
		}
		result = append(result, PatchInputRecord{
			PatchID: patch.PatchID, ProposalFingerprint: fingerprint,
			BundleFingerprint: patch.BundleFingerprint, OperationIDs: operationIDs,
		})
	}
	return result, nil
}

func assemblyInputsFor(componentMap *model.ComponentMap, originID string) (AssemblyInputs, error) {
	inputs := AssemblyInputs{}
	if componentMap == nil {
		return inputs, nil
	}
	raw, err := json.Marshal(componentMap)
	if err != nil {
		return AssemblyInputs{}, fmt.Errorf("encode component-map assembly input: %w", err)
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return AssemblyInputs{}, fmt.Errorf("canonicalize component-map assembly input: %w", err)
	}
	if strings.TrimSpace(originID) == "" {
		originID = "component-map"
	}
	var value model.ComponentMap
	if err := strictDecode(canonical, &value); err != nil {
		return AssemblyInputs{}, fmt.Errorf("decode canonical component-map assembly input: %w", err)
	}
	if value.Provenance != nil && value.Provenance.Repos == nil {
		value.Provenance.Repos = map[string]model.ComponentMapRepo{}
		raw, err = json.Marshal(value)
		if err != nil {
			return AssemblyInputs{}, fmt.Errorf("encode normalized component-map assembly input: %w", err)
		}
		canonical, err = canonicalJSON(raw)
		if err != nil {
			return AssemblyInputs{}, fmt.Errorf("canonicalize normalized component-map assembly input: %w", err)
		}
	}
	inputs.ComponentMap = &ComponentMapInput{
		ContentFingerprint: digest(canonical),
		Origin:             AssemblyInputOrigin{Kind: "orchestrator", ID: originID},
		Value:              value,
	}
	return inputs, nil
}

func decodeAnalyzer(raw []byte) ([]byte, map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, nil, fmt.Errorf("decode analyzer JSON: %w", err)
	}
	known := map[string]bool{}
	for key := range metadataFields {
		known[key] = true
	}
	for _, descriptor := range factRegistry {
		known[descriptor.Top] = true
	}
	for key := range top {
		if !known[key] {
			return nil, nil, fmt.Errorf("unsupported analyzer fact or field %q", key)
		}
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize analyzer JSON: %w", err)
	}
	return canonical, top, nil
}

func decompose(top map[string]json.RawMessage, revision string) ([]Fact, error) {
	var facts []Fact
	occurrences := map[string]int{}
	for _, descriptor := range factRegistry {
		raw, present := top[descriptor.Top]
		if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var values []factValue
		var err error
		switch descriptor.Shape {
		case shapeSingleton:
			values = []factValue{{Raw: raw, Pointer: "/" + descriptor.Top}}
		case shapeArray:
			values, err = arrayValues(raw, "/"+descriptor.Top)
		case shapeNestedSingleton, shapeNestedArray:
			values, err = nestedValues(raw, descriptor)
		case shapeMap, shapeMapArray:
			values, err = mapValues(raw, descriptor)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid analyzer field %s: %w", descriptor.Top, err)
		}
		for ordinal, value := range values {
			canonical, err := canonicalValue(value.Raw)
			if err != nil {
				return nil, fmt.Errorf("invalid %s fact at %s: %w", descriptor.Type, value.Pointer, err)
			}
			if err := validateFactValue(descriptor, canonical); err != nil {
				return nil, fmt.Errorf("invalid %s fact at %s: %w", descriptor.Type, value.Pointer, err)
			}
			baseID := factID(descriptor.Type, value.Key, canonical)
			occurrences[baseID]++
			id := baseID
			if occurrences[baseID] > 1 {
				id += ":" + strconv.Itoa(occurrences[baseID])
			}
			evidence, err := evidenceFrom(canonical, revision, descriptor.Type)
			if err != nil {
				return nil, fmt.Errorf("invalid %s fact evidence at %s: %w", descriptor.Type, value.Pointer, err)
			}
			facts = append(facts, Fact{
				ID: id, Type: descriptor.Type, Key: value.Key, Ordinal: ordinal,
				Value: canonical, InputPointer: value.Pointer,
				Evidence:    evidence,
				Uncertainty: uncertaintyFrom(canonical),
				Authority:   FactAuthority{Kind: "analyzer", Origin: "analyzer-input", ClaimClass: "implementation"},
			})
		}
	}
	return facts, nil
}

type factValue struct {
	Raw     json.RawMessage
	Key     string
	Pointer string
}

func arrayValues(raw json.RawMessage, pointer string) ([]factValue, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("expected array")
	}
	result := make([]factValue, 0, len(items))
	for index, item := range items {
		result = append(result, factValue{Raw: item, Pointer: pointer + "/" + strconv.Itoa(index)})
	}
	return result, nil
}

func nestedValues(raw json.RawMessage, descriptor factDescriptor) ([]factValue, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.New("expected object")
	}
	allowed := map[string]bool{}
	for _, candidate := range factRegistry {
		if candidate.Top == descriptor.Top && candidate.Child != "" {
			allowed[candidate.Child] = true
		}
	}
	for key := range object {
		if !allowed[key] {
			return nil, fmt.Errorf("unsupported nested field %q", key)
		}
	}
	value, present := object[descriptor.Child]
	if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, nil
	}
	pointer := "/" + descriptor.Top + "/" + descriptor.Child
	if descriptor.Shape == shapeNestedArray {
		return arrayValues(value, pointer)
	}
	return []factValue{{Raw: value, Pointer: pointer}}, nil
}

func mapValues(raw json.RawMessage, descriptor factDescriptor) ([]factValue, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.New("expected object")
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result []factValue
	for _, key := range keys {
		pointer := "/" + descriptor.Top + "/" + escapePointer(key)
		if descriptor.Shape == shapeMapArray {
			items, err := arrayValues(object[key], pointer)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			for index := range items {
				items[index].Key = key
			}
			result = append(result, items...)
			continue
		}
		result = append(result, factValue{Raw: object[key], Key: key, Pointer: pointer})
	}
	return result, nil
}

func compose(identity model.Input, facts []Fact) (model.Input, error) {
	top := map[string]any{
		"component": identity.Component, "repo": identity.Repo,
		"commit_sha": identity.CommitSHA, "extracted_at": identity.ExtractedAt,
		"analyzer_version": identity.AnalyzerVersion, "schema_version": identity.SchemaVersion,
	}
	groups := map[string][]Fact{}
	for _, fact := range facts {
		groups[fact.Type] = append(groups[fact.Type], fact)
	}
	for _, descriptor := range factRegistry {
		items := groups[descriptor.Type]
		sort.SliceStable(items, func(i, j int) bool { return items[i].Ordinal < items[j].Ordinal })
		if len(items) == 0 {
			continue
		}
		decode := func(raw json.RawMessage) (any, error) {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			return value, nil
		}
		switch descriptor.Shape {
		case shapeSingleton:
			value, _ := decode(items[0].Value)
			top[descriptor.Top] = value
		case shapeArray:
			array := make([]any, 0, len(items))
			for _, fact := range items {
				value, _ := decode(fact.Value)
				array = append(array, value)
			}
			top[descriptor.Top] = array
		case shapeNestedSingleton, shapeNestedArray:
			object, _ := top[descriptor.Top].(map[string]any)
			if object == nil {
				object = map[string]any{}
			}
			if descriptor.Shape == shapeNestedSingleton {
				value, _ := decode(items[0].Value)
				object[descriptor.Child] = value
			} else {
				array := make([]any, 0, len(items))
				for _, fact := range items {
					value, _ := decode(fact.Value)
					array = append(array, value)
				}
				object[descriptor.Child] = array
			}
			top[descriptor.Top] = object
		case shapeMap, shapeMapArray:
			object, _ := top[descriptor.Top].(map[string]any)
			if object == nil {
				object = map[string]any{}
			}
			if descriptor.Shape == shapeMap {
				for _, fact := range items {
					value, _ := decode(fact.Value)
					object[fact.Key] = value
				}
			} else {
				byKey := map[string][]Fact{}
				for _, fact := range items {
					byKey[fact.Key] = append(byKey[fact.Key], fact)
				}
				for key, keyed := range byKey {
					sort.SliceStable(keyed, func(i, j int) bool { return keyed[i].Ordinal < keyed[j].Ordinal })
					array := make([]any, 0, len(keyed))
					for _, fact := range keyed {
						value, _ := decode(fact.Value)
						array = append(array, value)
					}
					object[key] = array
				}
			}
			top[descriptor.Top] = object
		}
	}
	raw, err := json.Marshal(top)
	if err != nil {
		return model.Input{}, fmt.Errorf("compose analyzer facts: %w", err)
	}
	var result model.Input
	if err := strictDecode(raw, &result); err != nil {
		return model.Input{}, fmt.Errorf("compose analyzer facts: %w", err)
	}
	return result, nil
}

func validateFactValue(descriptor factDescriptor, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return errors.New("empty value")
	}
	scalar := descriptor.Type == "summary" || descriptor.Type == "dependency_go_version" || descriptor.Type == "data_coverage"
	if scalar && trimmed[0] != '"' {
		return errors.New("expected string value")
	}
	if !scalar && trimmed[0] != '{' {
		return errors.New("expected object value")
	}
	value := descriptor.NewValue()
	if err := strictDecode(raw, value); err != nil {
		return err
	}
	return nil
}

func strictDecode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func canonicalValue(raw json.RawMessage) (json.RawMessage, error) {
	return canonicalJSON(raw)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("unexpected trailing JSON value")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func factID(factType, key string, raw json.RawMessage) string {
	sum := sha256.Sum256(bytes.Join([][]byte{[]byte(factType), []byte(key), raw}, []byte{0}))
	return "fact:" + factType + ":" + hex.EncodeToString(sum[:])
}

func accountingFor(facts []Fact) []FactAccounting {
	result := make([]FactAccounting, 0, len(facts))
	for _, fact := range facts {
		descriptor, _ := descriptorByType(fact.Type)
		disposition := "retained"
		if descriptor.Rendered && fact.Authority.ClaimClass != "planned" && fact.Authority.ClaimClass != "support" {
			disposition = "rendered"
		}
		result = append(result, FactAccounting{
			FactID: fact.ID, InputPointer: fact.InputPointer,
			Disposition: disposition, Sections: append([]string{}, descriptor.Sections...),
		})
	}
	return result
}

func evidenceFrom(raw json.RawMessage, revision, factType string) ([]EvidenceRef, error) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return []EvidenceRef{}, nil
	}
	seen := map[string]bool{}
	result := []EvidenceRef{}
	var evidenceErr error
	recordSource := func(source string) {
		for _, part := range strings.Split(source, ";") {
			part = strings.TrimSpace(part)
			if part == "" || !sourceCandidate(part) {
				continue
			}
			path, start, end, err := splitEvidence(part)
			if err != nil {
				evidenceErr = err
				return
			}
			appendEvidence(&result, seen, path, start, end, revision)
		}
	}
	var walk func(any)
	walk = func(current any) {
		if evidenceErr != nil {
			return
		}
		switch typed := current.(type) {
		case map[string]any:
			if file, ok := typed["file"].(string); ok {
				line := intValue(typed["line"])
				appendEvidence(&result, seen, file, line, line, revision)
			}
			for key, item := range typed {
				switch key {
				case "source":
					if source, ok := item.(string); ok {
						recordSource(source)
					}
				case "sources":
					if sources, ok := item.([]any); ok {
						for _, rawSource := range sources {
							if source, ok := rawSource.(string); ok {
								recordSource(source)
							}
						}
					}
				}
				walk(item)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	if evidenceErr != nil {
		return nil, evidenceErr
	}
	if factType == "dockerfile" {
		if object, ok := value.(map[string]any); ok {
			if path, ok := object["path"].(string); ok {
				appendEvidence(&result, seen, path, 0, 0, revision)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path+fmt.Sprintf(":%09d:%09d", result[i].StartLine, result[i].EndLine) <
			result[j].Path+fmt.Sprintf(":%09d:%09d", result[j].StartLine, result[j].EndLine)
	})
	return result, nil
}

func appendEvidence(result *[]EvidenceRef, seen map[string]bool, path string, start, end int, revision string) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return
	}
	key := fmt.Sprintf("%s:%d:%d", path, start, end)
	if seen[key] {
		return
	}
	seen[key] = true
	*result = append(*result, EvidenceRef{Path: path, StartLine: start, EndLine: end, Revision: valueOr(revision, "unknown")})
}

func sourceCandidate(source string) bool {
	path, _, _, _ := splitEvidence(source)
	base := filepath.Base(path)
	return strings.Contains(path, "/") || strings.Contains(base, ".") || strings.HasPrefix(base, "Dockerfile") || base == "Makefile"
}

func splitEvidence(source string) (string, int, int, error) {
	path := strings.TrimSpace(source)
	position := strings.LastIndex(path, ":")
	if position < 0 {
		return path, 0, 0, nil
	}
	rangeValue := path[position+1:]
	parts := strings.Split(rangeValue, "-")
	if len(parts) < 1 || len(parts) > 2 {
		if len(rangeValue) > 0 && rangeValue[0] >= '0' && rangeValue[0] <= '9' {
			return "", 0, 0, fmt.Errorf("malformed source range %q", source)
		}
		return path, 0, 0, nil
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 1 {
		if len(rangeValue) > 0 && rangeValue[0] >= '0' && rangeValue[0] <= '9' {
			return "", 0, 0, fmt.Errorf("malformed source range %q", source)
		}
		return path, 0, 0, nil
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil || end < start {
			return "", 0, 0, fmt.Errorf("malformed source range %q", source)
		}
	}
	return path[:position], start, end, nil
}

func intValue(value any) int {
	if number, ok := value.(float64); ok && number > 0 {
		return int(number)
	}
	return 0
}

func uncertaintyFrom(raw json.RawMessage) FactUncertainty {
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return FactUncertainty{Status: "extracted"}
	}
	status, _ := object["status"].(string)
	switch status {
	case "unknown", "not-extracted", "unresolved", "planned", "not-verified":
		return FactUncertainty{Status: status, Detail: "Preserved analyzer status; no stronger claim was inferred."}
	default:
		return FactUncertainty{Status: "extracted"}
	}
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func normalizedStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
