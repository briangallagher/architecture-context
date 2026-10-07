// Command arch-doc assembles architecture Markdown using explicit section ownership.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed section-manifest.json
var manifestBytes []byte

type manifest struct {
	RequiredSections         []string            `json:"required_sections"`
	KnownSections            []string            `json:"known_sections"`
	AnalyzerSections         []string            `json:"analyzer_sections"`
	SynthesisSections        []string            `json:"synthesis_sections"`
	ConditionalSynthesis     []string            `json:"conditional_synthesis_sections"`
	SharedSections           []string            `json:"shared_sections"`
	SynthesisSubsections     map[string][]string `json:"synthesis_subsections"`
	NonAuthoritativeSections []string            `json:"non_authoritative_sections"`
}

type section struct {
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Text      string `json:"-"`
	StartLine int    `json:"-"`
}

type document struct {
	Preamble  string
	Sections  []section
	Duplicate []string
}

type sectionDecision struct {
	Status         string `json:"status"`
	Subsection     string `json:"subsection"`
	ExpectedParent string `json:"expected_parent"`
	ActualParent   string `json:"actual_parent"`
	Line           int    `json:"line"`
	Detail         string `json:"detail"`
}

type assemblyDiagnostic struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Document       string `json:"document"`
	Subsection     string `json:"subsection,omitempty"`
	ExpectedParent string `json:"expected_parent,omitempty"`
	ActualParent   string `json:"actual_parent,omitempty"`
	Line           int    `json:"line,omitempty"`
	Message        string `json:"message"`
}

type assemblyReport struct {
	SchemaVersion    int                  `json:"schema_version"`
	Status           string               `json:"status"`
	SectionDecisions []sectionDecision    `json:"section_decisions"`
	Diagnostics      []assemblyDiagnostic `json:"diagnostics"`
}

var config manifest

func init() {
	if err := json.Unmarshal(manifestBytes, &config); err != nil {
		panic(fmt.Sprintf("invalid embedded section manifest: %v", err))
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "sections":
		exit(runSections(os.Args[2:]))
	case "validate":
		exit(runValidate(os.Args[2:]))
	case "update":
		exit(runUpdate(os.Args[2:]))
	case "assemble":
		exit(runAssemble(os.Args[2:]))
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  arch-doc sections FILE [--output text|json]")
	fmt.Fprintln(os.Stderr, "  arch-doc validate FILE")
	fmt.Fprintln(os.Stderr, "  arch-doc update FILE --section NAME --input CONTENT [--output FILE]")
	fmt.Fprintln(os.Stderr, "  arch-doc assemble --base FILE --candidate FILE --output FILE")
	os.Exit(2)
}

func exit(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "arch-doc:", err)
	os.Exit(1)
}

func runSections(args []string) error {
	flags := flag.NewFlagSet("sections", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "text", "output format: text or json")
	if err := flags.Parse(reorderPositionalFile(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("sections requires exactly one FILE")
	}
	doc, err := readDocument(flags.Arg(0))
	if err != nil {
		return err
	}
	if *output == "json" {
		payload := struct {
			Sections  []section `json:"sections"`
			Duplicate []string  `json:"duplicate_sections,omitempty"`
		}{doc.Sections, doc.Duplicate}
		encoded, encodeErr := json.MarshalIndent(payload, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		fmt.Println(string(encoded))
		return nil
	}
	if *output != "text" {
		return fmt.Errorf("unsupported output format %q", *output)
	}
	for _, item := range doc.Sections {
		fmt.Printf("%-28s %s\n", item.Name, item.Owner)
	}
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderPositionalFile(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("validate requires exactly one FILE")
	}
	doc, err := readDocument(flags.Arg(0))
	if err != nil {
		return err
	}
	errorsFound := validateDocument(doc)
	if len(errorsFound) != 0 {
		return errors.New(strings.Join(errorsFound, "; "))
	}
	fmt.Printf("Valid architecture document: %s (%d sections)\n", flags.Arg(0), len(doc.Sections))
	return nil
}

func runUpdate(args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sectionName := flags.String("section", "", "section to replace")
	inputPath := flags.String("input", "", "file containing section content")
	outputPath := flags.String("output", "", "output file; defaults to in-place")
	if err := flags.Parse(reorderPositionalFile(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *sectionName == "" || *inputPath == "" {
		return errors.New("update requires FILE, --section, and --input")
	}
	owner := ownerFor(*sectionName)
	if owner != "synthesis" {
		return fmt.Errorf("section %q is owned by %s and cannot be updated by an agent", *sectionName, owner)
	}
	doc, err := readDocument(flags.Arg(0))
	if err != nil {
		return err
	}
	if validation := validateDocument(doc); len(validation) > 0 {
		return errors.New(strings.Join(validation, "; "))
	}
	content, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read update content: %w", err)
	}
	replacement := normalizeSectionContent(*sectionName, string(content))
	updated, err := replaceSection(doc, *sectionName, replacement, true)
	if err != nil {
		return err
	}
	updatedText := renderDocument(updated)
	if validation := validateDocument(parseDocument(updatedText)); len(validation) > 0 {
		return errors.New(strings.Join(validation, "; "))
	}
	target := flags.Arg(0)
	if *outputPath != "" {
		target = *outputPath
	}
	return atomicWrite(target, updatedText)
}

func runAssemble(args []string) error {
	flags := flag.NewFlagSet("assemble", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	basePath := flags.String("base", "", "table-merged analyzer base")
	candidatePath := flags.String("candidate", "", "agent candidate")
	outputPath := flags.String("output", "", "assembled output")
	reportPath := flags.String("report", "", "machine-readable assembly report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *basePath == "" || *candidatePath == "" || *outputPath == "" {
		return errors.New("assemble requires --base, --candidate, and --output")
	}
	base, err := readDocument(*basePath)
	if err != nil {
		return fmt.Errorf("read base: %w", err)
	}
	candidate, err := readDocument(*candidatePath)
	if err != nil {
		return fmt.Errorf("read candidate: %w", err)
	}
	if validation := validateDocument(base); len(validation) > 0 {
		return errors.New(strings.Join(validation, "; "))
	}
	if validation := validateSynthesisInput(candidate, "candidate"); len(validation) > 0 {
		return errors.New(strings.Join(validation, "; "))
	}
	assembled, decisions, diagnostics, err := assembleDocumentsWithReport(base, candidate)
	if err != nil {
		if reportErr := writeAssemblyReport(*reportPath, assemblyReport{
			SchemaVersion: 1, Status: "failed", SectionDecisions: decisions,
			Diagnostics: diagnostics,
		}); reportErr != nil {
			return fmt.Errorf("%v; write assembly report: %w", err, reportErr)
		}
		return err
	}
	if validation := validateDocument(assembled); len(validation) > 0 {
		err := errors.New(strings.Join(validation, "; "))
		if reportErr := writeAssemblyReport(*reportPath, assemblyReport{
			SchemaVersion: 1, Status: "failed", SectionDecisions: decisions,
			Diagnostics: []assemblyDiagnostic{{
				Code: "assembled_document_invalid", Severity: "error",
				Document: "assembled", Message: err.Error(),
			}},
		}); reportErr != nil {
			return fmt.Errorf("%v; write assembly report: %w", err, reportErr)
		}
		return err
	}
	if err := atomicWrite(*outputPath, renderDocument(assembled)); err != nil {
		return err
	}
	return writeAssemblyReport(*reportPath, assemblyReport{
		SchemaVersion: 1, Status: "success", SectionDecisions: decisions,
		Diagnostics: []assemblyDiagnostic{},
	})
}

func readDocument(path string) (document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return document{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parseDocument(string(data)), nil
}

func parseDocument(text string) document {
	lines := strings.SplitAfter(text, "\n")
	var current *section
	var preamble strings.Builder
	var sections []section
	seen := map[string]bool{}
	duplicates := []string{}
	for lineIndex, line := range lines {
		if strings.HasPrefix(line, "## ") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line[3:], "\n"), "\r"))
			if seen[name] {
				duplicates = appendUnique(duplicates, name)
			}
			seen[name] = true
			sections = append(sections, section{Name: name, Owner: ownerFor(name), StartLine: lineIndex + 1})
			current = &sections[len(sections)-1]
			current.Text = line
			continue
		}
		if current == nil {
			preamble.WriteString(line)
		} else {
			current.Text += line
		}
	}
	return document{Preamble: preamble.String(), Sections: sections, Duplicate: duplicates}
}

func renderDocument(doc document) string {
	var output strings.Builder
	output.WriteString(doc.Preamble)
	for _, item := range doc.Sections {
		output.WriteString(item.Text)
	}
	return output.String()
}

func validateDocument(doc document) []string {
	issues := []string{}
	for _, duplicate := range doc.Duplicate {
		issues = append(issues, fmt.Sprintf("duplicate section: %s", duplicate))
	}
	present := map[string]bool{}
	for _, item := range doc.Sections {
		present[item.Name] = true
		if !contains(config.KnownSections, item.Name) {
			issues = append(issues, "unknown section: "+item.Name)
		}
	}
	for _, required := range config.RequiredSections {
		if !present[required] {
			issues = append(issues, "missing required section: "+required)
		}
	}
	return issues
}

func validateSynthesisInput(doc document, label string) []string {
	issues := []string{}
	for _, duplicate := range doc.Duplicate {
		issues = append(issues, fmt.Sprintf("%s duplicate section: %s", label, duplicate))
	}
	present := map[string]bool{}
	for _, item := range doc.Sections {
		present[item.Name] = true
		if !contains(config.KnownSections, item.Name) {
			issues = append(issues, fmt.Sprintf("%s unknown section: %s", label, item.Name))
		}
	}
	for _, required := range config.SynthesisSections {
		if !present[required] {
			issues = append(issues, fmt.Sprintf("%s missing synthesis section: %s", label, required))
		}
	}
	return issues
}

func assembleDocuments(base, candidate document) (document, error) {
	assembled, _, _, err := assembleDocumentsWithReport(base, candidate)
	return assembled, err
}

func assembleDocumentsWithReport(base, candidate document) (document, []sectionDecision, []assemblyDiagnostic, error) {
	diagnostics := validateSynthesisSubsectionPlacement(candidate)
	if len(diagnostics) > 0 {
		messages := make([]string, 0, len(diagnostics))
		for _, diagnostic := range diagnostics {
			messages = append(messages, diagnostic.Message)
		}
		return document{}, nil, diagnostics, errors.New(strings.Join(messages, "; "))
	}
	result := base
	candidateByName := sectionMap(candidate.Sections)
	for _, name := range config.SynthesisSections {
		candidateSection, ok := candidateByName[name]
		if !ok {
			return document{}, nil, nil, fmt.Errorf("candidate missing synthesis section: %s", name)
		}
		if _, ok := sectionMap(result.Sections)[name]; !ok {
			return document{}, nil, nil, fmt.Errorf("base missing synthesis section: %s", name)
		}
		result, _ = replaceSection(result, name, candidateSection.Text, false)
	}
	var decisions []sectionDecision
	result, decisions = mergeSynthesisSubsections(result, candidateByName)
	for _, name := range config.ConditionalSynthesis {
		candidateSection, ok := candidateByName[name]
		if !ok || hasSection(result, name) {
			continue
		}
		result.Sections = append(result.Sections, candidateSection)
	}
	if generated := generatedBy(candidate); generated != "" {
		result = replaceGeneratedBy(result, generated)
	}
	return result, decisions, nil, nil
}

func mergeSynthesisSubsections(base document, candidate map[string]section) (document, []sectionDecision) {
	parents := make([]string, 0, len(config.SynthesisSubsections))
	for parent := range config.SynthesisSubsections {
		parents = append(parents, parent)
	}
	sort.Strings(parents)
	decisions := []sectionDecision{}
	for _, parent := range parents {
		baseParent, baseOK := findSection(base, parent)
		candidateParent, candidateOK := candidate[parent]
		if !baseOK || !candidateOK {
			continue
		}
		parentText := baseParent.Text
		for _, subsection := range config.SynthesisSubsections[parent] {
			block, line := extractSubsection(candidateParent, subsection)
			if block == "" {
				continue
			}
			decision := sectionDecision{
				Subsection: subsection, ExpectedParent: parent,
				ActualParent: parent, Line: line,
			}
			if existing, _ := extractSubsection(baseParent, subsection); existing != "" {
				decision.Status = "discarded"
				decision.Detail = "base subsection retained; candidate subsection was not promoted"
			} else {
				parentText = strings.TrimRight(parentText, "\r\n") + "\n\n" + strings.TrimRight(block, "\r\n") + "\n"
				decision.Status = "applied"
				decision.Detail = "candidate subsection added under its configured parent"
			}
			decisions = append(decisions, decision)
		}
		updated, _ := replaceSection(base, parent, parentText, false)
		base = updated
	}
	return base, decisions
}

func extractSubsection(parent section, name string) (string, int) {
	lines := strings.SplitAfter(parent.Text, "\n")
	needle := "### " + name
	start, end := -1, len(lines)
	fence := ""
	for index, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(strings.TrimSuffix(line, "\n"), "\r"))
		if marker := fenceMarker(trimmed); marker != "" {
			if fence == "" {
				fence = marker
			} else if strings.HasPrefix(marker, fence) {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			if start >= 0 {
				end = index
				break
			}
			if trimmed == needle {
				start = index
			}
		}
	}
	if start < 0 {
		return "", 0
	}
	return strings.Join(lines[start:end], ""), parent.StartLine + start
}

func validateSynthesisSubsectionPlacement(candidate document) []assemblyDiagnostic {
	expected := map[string]string{}
	for parent, subsections := range config.SynthesisSubsections {
		for _, subsection := range subsections {
			expected[subsection] = parent
		}
	}
	diagnostics := []assemblyDiagnostic{}
	check := func(parent section) {
		for subsection, expectedParent := range expected {
			if _, line := extractSubsection(parent, subsection); line != 0 && parent.Name != expectedParent {
				message := fmt.Sprintf(
					"candidate synthesis subsection %q is under %q at line %d; expected parent %q",
					subsection, parent.Name, line, expectedParent,
				)
				diagnostics = append(diagnostics, assemblyDiagnostic{
					Code: "synthesis_subsection_parent_mismatch", Severity: "error",
					Document: "candidate", Subsection: subsection,
					ExpectedParent: expectedParent, ActualParent: parent.Name,
					Line: line, Message: message,
				})
			}
		}
	}
	if candidate.Preamble != "" {
		check(section{Name: "<document root>", Text: candidate.Preamble, StartLine: 1})
	}
	for _, parent := range candidate.Sections {
		check(parent)
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Line != diagnostics[j].Line {
			return diagnostics[i].Line < diagnostics[j].Line
		}
		return diagnostics[i].Subsection < diagnostics[j].Subsection
	})
	return diagnostics
}

func fenceMarker(line string) string {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return ""
	}
	length := 0
	for length < len(line) && line[length] == line[0] {
		length++
	}
	if length < 3 {
		return ""
	}
	return line[:length]
}

func writeAssemblyReport(path string, report assemblyReport) error {
	if path == "" {
		return nil
	}
	if report.SectionDecisions == nil {
		report.SectionDecisions = []sectionDecision{}
	}
	if report.Diagnostics == nil {
		report.Diagnostics = []assemblyDiagnostic{}
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(encoded)+"\n")
}

func replaceSection(doc document, name, replacement string, allowAppend bool) (document, error) {
	for index := range doc.Sections {
		if doc.Sections[index].Name == name {
			doc.Sections[index].Text = replacement
			return doc, nil
		}
	}
	if !allowAppend {
		return doc, fmt.Errorf("section not found: %s", name)
	}
	doc.Sections = append(doc.Sections, section{Name: name, Owner: ownerFor(name), Text: replacement})
	return doc, nil
}

func normalizeSectionContent(name, content string) string {
	content = strings.TrimSpace(content)
	prefix := "## " + name
	if strings.HasPrefix(content, prefix) {
		return content + "\n"
	}
	return prefix + "\n\n" + content + "\n"
}

func replaceGeneratedBy(doc document, line string) document {
	for index := range doc.Sections {
		lines := strings.SplitAfter(doc.Sections[index].Text, "\n")
		for lineIndex, current := range lines {
			if strings.HasPrefix(strings.TrimSpace(current), "- **Generated By**:") {
				lines[lineIndex] = line + "\n"
			}
		}
		doc.Sections[index].Text = strings.Join(lines, "")
	}
	return doc
}

func generatedBy(doc document) string {
	for _, item := range doc.Sections {
		for _, line := range strings.Split(item.Text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "- **Generated By**:") {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

func sectionMap(sections []section) map[string]section {
	result := make(map[string]section, len(sections))
	for _, item := range sections {
		result[item.Name] = item
	}
	return result
}

func findSection(doc document, name string) (section, bool) {
	for _, item := range doc.Sections {
		if item.Name == name {
			return item, true
		}
	}
	return section{}, false
}

func hasSection(doc document, name string) bool {
	_, ok := findSection(doc, name)
	return ok
}

func ownerFor(name string) string {
	for _, item := range config.SynthesisSections {
		if item == name {
			return "synthesis"
		}
	}
	for _, item := range config.ConditionalSynthesis {
		if item == name {
			return "synthesis-conditional"
		}
	}
	for _, item := range config.SharedSections {
		if item == name {
			return "shared"
		}
	}
	for _, item := range config.AnalyzerSections {
		if item == name {
			return "analyzer"
		}
	}
	for _, item := range config.NonAuthoritativeSections {
		if item == name {
			return "non-authoritative"
		}
	}
	for _, item := range config.RequiredSections {
		if item == name {
			return "analyzer"
		}
	}
	return "unmanaged"
}

func atomicWrite(path, content string) error {
	target := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".arch-doc-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func reorderPositionalFile(args []string) []string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args
	}
	return append(append([]string{}, args[1:]...), args[0])
}
