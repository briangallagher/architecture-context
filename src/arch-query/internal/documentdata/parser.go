// Package documentdata validates and maps accepted structured component
// documents. The rendering_view is the compatibility authority; typed facts
// are consulted only for fields that rendering_view does not expose.
package documentdata

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
	analyzerdocument "github.com/jctanner/arch-analyzer/pkg/document"
	"github.com/jctanner/arch-query/internal/types"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	documentSchemaVersion   = "1.0.0/1.1.0"
	schemaResource          = "https://github.com/jctanner/odh.architecture-context/schemas/structured-component-document-v1.schema.json"
	synthesisSchemaResource = "https://github.com/jctanner/odh.architecture-context/schemas/structured-component-synthesis-envelope-v1.schema.json"
	synthesisListLimit      = 4
)

//go:embed structured-component-document-v1.schema.json
var documentSchemaJSON []byte

//go:embed structured-component-synthesis-envelope-v1.schema.json
var synthesisSchemaJSON []byte

var (
	compileSchemaOnce       sync.Once
	compiledSchema          *jsonschema.Schema
	compileSchemaErr        error
	compileSynthesisOnce    sync.Once
	compiledSynthesisSchema *jsonschema.Schema
	compileSynthesisErr     error
)

type acceptedDocument struct {
	SchemaVersion string `json:"schema_version"`
	Identity      struct {
		Component         string   `json:"component"`
		SourceComponent   string   `json:"source_component"`
		Repository        string   `json:"repository"`
		SourceRevision    string   `json:"source_revision"`
		VersionScope      string   `json:"version_scope"`
		IntegrationStatus string   `json:"integration_status"`
		Aliases           []string `json:"aliases"`
	} `json:"identity"`
	Producers struct {
		AnalyzerVersion string `json:"analyzer_version"`
	} `json:"producers"`
	AnalyzerInput struct {
		BundleFingerprint string `json:"bundle_fingerprint"`
	} `json:"analyzer_input"`
	Facts          []fact           `json:"facts"`
	FactAccounting []factAccounting `json:"fact_accounting"`
	Sections       []section        `json:"sections"`
	PatchInputs    []patchInput     `json:"patch_inputs"`
	Dispositions   []disposition    `json:"proposal_dispositions"`
	Uncertainty    []uncertainty    `json:"uncertainty"`
	RenderingView  renderingView    `json:"rendering_view"`
}

type evidence struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Revision  string `json:"revision"`
}

type authority struct {
	Kind       string `json:"kind"`
	Origin     string `json:"origin"`
	ClaimClass string `json:"claim_class"`
}

type fact struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Key          string          `json:"key,omitempty"`
	Ordinal      int             `json:"ordinal"`
	Value        json.RawMessage `json:"value"`
	InputPointer string          `json:"input_pointer"`
	Evidence     []evidence      `json:"evidence"`
	Authority    authority       `json:"authority"`
}

type factAccounting struct {
	FactID       string   `json:"fact_id"`
	InputPointer string   `json:"input_pointer"`
	Disposition  string   `json:"disposition"`
	Sections     []string `json:"sections"`
}

type section struct {
	ID        string     `json:"id"`
	Evidence  []evidence `json:"evidence"`
	Blocks    []block    `json:"blocks"`
	Authority authority  `json:"authority"`
}

type block struct {
	TableID string     `json:"table_id,omitempty"`
	Rows    []tableRow `json:"rows,omitempty"`
}

type tableRow struct {
	Evidence []evidence `json:"evidence"`
}

type patchInput struct {
	PatchID             string   `json:"patch_id"`
	ProposalFingerprint string   `json:"proposal_fingerprint"`
	BundleFingerprint   string   `json:"bundle_fingerprint"`
	OperationIDs        []string `json:"operation_ids"`
}

type disposition struct {
	PatchID             string     `json:"patch_id"`
	ProposalFingerprint string     `json:"proposal_fingerprint"`
	BundleFingerprint   string     `json:"bundle_fingerprint"`
	OperationID         string     `json:"operation_id"`
	Action              string     `json:"action"`
	FactType            string     `json:"fact_type"`
	OriginKind          string     `json:"origin_kind"`
	OriginID            string     `json:"origin_id"`
	ClaimClass          string     `json:"claim_class"`
	Status              string     `json:"status"`
	TargetFactID        string     `json:"target_fact_id,omitempty"`
	ResultingFactID     string     `json:"resulting_fact_id,omitempty"`
	Evidence            []evidence `json:"evidence"`
	ProposalReason      string     `json:"proposal_reason"`
	PolicyID            string     `json:"policy_id"`
	AuthorizedBy        string     `json:"authorized_by"`
	AllowedFactTypes    []string   `json:"allowed_fact_types"`
	DecisionID          string     `json:"decision_id"`
	DecidedBy           string     `json:"decided_by"`
	DecisionReason      string     `json:"decision_reason"`
}

type uncertainty struct {
	Scope  string `json:"scope"`
	Status string `json:"status"`
}

type renderingView struct {
	Component              string         `json:"component"`
	Metadata               renderMetadata `json:"metadata"`
	Purpose                string         `json:"purpose"`
	ArchitectureComponents []struct {
		Component string `json:"component"`
		Type      string `json:"type"`
		Purpose   string `json:"purpose"`
	} `json:"architecture_components"`
	CRDs []struct {
		Group   string `json:"group"`
		Version string `json:"version"`
		Kind    string `json:"kind"`
		Scope   string `json:"scope"`
		APIRole string `json:"api_role"`
		Purpose string `json:"purpose"`
	} `json:"crds"`
	ServingRuntimes []map[string]any `json:"serving_runtimes"`
	HTTPEndpoints   []struct {
		Path       string `json:"path"`
		Method     string `json:"method"`
		Port       string `json:"port"`
		Protocol   string `json:"protocol"`
		Transport  string `json:"transport"`
		Encryption string `json:"encryption"`
		Auth       string `json:"auth"`
		Owner      string `json:"owner"`
		Purpose    string `json:"purpose"`
	} `json:"http_endpoints"`
	GRPCServices []struct {
		Service    string `json:"service"`
		Port       string `json:"port"`
		Protocol   string `json:"protocol"`
		Transport  string `json:"transport"`
		Encryption string `json:"encryption"`
		Auth       string `json:"auth"`
		Owner      string `json:"owner"`
		Purpose    string `json:"purpose"`
	} `json:"grpc_services"`
	ExternalDependencies []struct {
		Component string `json:"component"`
		Version   string `json:"version"`
		Required  string `json:"required"`
		Role      string `json:"role"`
		Purpose   string `json:"purpose"`
	} `json:"external_dependencies"`
	InternalDependencies []struct {
		Component       string `json:"component"`
		InteractionType string `json:"interaction_type"`
		Role            string `json:"role"`
		Purpose         string `json:"purpose"`
	} `json:"internal_dependencies"`
	Services []types.Service `json:"services"`
	Ingress  []struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Hosts      string `json:"hosts"`
		Port       string `json:"port"`
		Protocol   string `json:"protocol"`
		Encryption string `json:"encryption"`
		TLSMode    string `json:"tls_mode"`
		Exposure   string `json:"exposure"`
	} `json:"ingress"`
	Egress       []types.Egress `json:"egress"`
	ClusterRoles []struct {
		Name            string `json:"name"`
		APIGroup        string `json:"api_group"`
		Resources       string `json:"resources"`
		NonResourceURLs string `json:"non_resource_urls"`
		Verbs           string `json:"verbs"`
	} `json:"cluster_roles"`
	Webhooks []struct {
		Name          string `json:"name"`
		Type          string `json:"type"`
		Path          string `json:"path"`
		Port          string `json:"port"`
		FailurePolicy string `json:"failure_policy"`
		Resources     string `json:"resources"`
		Operations    string `json:"operations"`
		Purpose       string `json:"purpose"`
	} `json:"webhooks"`
	IntegrationPoints    []map[string]any                        `json:"integration_points"`
	Sources              []sourceRow                             `json:"sources"`
	CrossCuttingEvidence map[string][]types.CrossCuttingEvidence `json:"cross_cutting_evidence"`
}

type renderMetadata struct {
	Repository     string `json:"repository"`
	Version        string `json:"version"`
	Distribution   string `json:"distribution"`
	Languages      string `json:"languages"`
	DeploymentType string `json:"deployment_type"`
	GeneratedBy    string `json:"generated_by"`
}

type sourceRow struct {
	File     string `json:"file"`
	Lines    string `json:"lines"`
	Sections string `json:"sections"`
}

// Parse reads, validates, and maps an accepted document. expectedComponent is
// the publication directory name and expectedVersion is its resolved version.
func Parse(fsys fs.FS, path, expectedComponent, expectedVersion string) (*types.ComponentDoc, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	if err := analyzerdocument.ValidateJSON(raw); err != nil {
		return nil, fmt.Errorf("validating accepted document %s with shared analyzer validator: %w", path, err)
	}
	instance, err := decodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing accepted document %s: %w", path, err)
	}
	object, ok := instance.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing accepted document %s: top-level value must be an object", path)
	}
	version, _ := object["schema_version"].(string)
	if version != "1.0.0" && version != "1.1.0" {
		return nil, fmt.Errorf("validating accepted document %s: unsupported schema_version %q (supported: %q)", path, version, documentSchemaVersion)
	}
	schema, err := acceptedSchema()
	if err != nil {
		return nil, fmt.Errorf("compile accepted document schema: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return nil, fmt.Errorf("validating accepted document %s against schema %s: %w", path, documentSchemaVersion, err)
	}
	var document acceptedDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decoding accepted document %s: %w", path, err)
	}
	if err := validateConsumerIdentity(document, expectedComponent, expectedVersion); err != nil {
		return nil, fmt.Errorf("validating accepted document %s: %w", path, err)
	}
	result, err := mapDocument(document)
	if err != nil {
		return nil, fmt.Errorf("mapping accepted document %s: %w", path, err)
	}
	result.FileName = expectedComponent + ".md"
	if version == "1.1.0" {
		componentDir := strings.TrimSuffix(path, "/document.json")
		versionDir := strings.TrimSuffix(componentDir, "/"+expectedComponent)
		analyzer, readErr := fs.ReadFile(fsys, componentDir+"/analyzer.json")
		if readErr != nil {
			return nil, fmt.Errorf("reading published analyzer for %s: %w", path, readErr)
		}
		synthesis, readErr := fs.ReadFile(fsys, componentDir+"/synthesis.json")
		if readErr != nil {
			return nil, fmt.Errorf("reading published synthesis for %s: %w", path, readErr)
		}
		if err := ValidateSynthesisJSON(synthesis); err != nil {
			return nil, fmt.Errorf("validating published synthesis schema for %s: %w", path, err)
		}
		if err := analyzerdocument.ValidateAuthorityJSON(raw, analyzer, synthesis); err != nil {
			return nil, fmt.Errorf("validating published authority %s: %w", path, err)
		}
		markdownPath := versionDir + "/" + expectedComponent + ".md"
		markdownData, readErr := fs.ReadFile(fsys, markdownPath)
		if readErr == nil {
			if err := analyzerdocument.ValidatePublicationJSON(raw, analyzer, synthesis, markdownData); err != nil {
				return nil, fmt.Errorf("validating published derivative %s: %w", markdownPath, err)
			}
		} else if errors.Is(readErr, fs.ErrNotExist) {
			result.FileName = ""
		} else {
			return nil, fmt.Errorf("reading published derivative %s: %w", markdownPath, readErr)
		}
	}
	return result, nil
}

// ValidateSynthesisJSON checks the central synthesis-envelope schema embedded
// in arch-query. Release staging uses the same copy as the query loader.
func ValidateSynthesisJSON(raw []byte) error {
	instance, err := decodeJSON(raw)
	if err != nil {
		return err
	}
	compileSynthesisOnce.Do(func() {
		schemaValue, decodeErr := decodeJSON(synthesisSchemaJSON)
		if decodeErr != nil {
			compileSynthesisErr = decodeErr
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.UseRegexpEngine(compileECMARegexp)
		if addErr := compiler.AddResource(synthesisSchemaResource, schemaValue); addErr != nil {
			compileSynthesisErr = addErr
			return
		}
		compiledSynthesisSchema, compileSynthesisErr = compiler.Compile(synthesisSchemaResource)
	})
	if compileSynthesisErr != nil {
		return compileSynthesisErr
	}
	return compiledSynthesisSchema.Validate(instance)
}

func acceptedSchema() (*jsonschema.Schema, error) {
	compileSchemaOnce.Do(func() {
		instance, err := decodeJSON(documentSchemaJSON)
		if err != nil {
			compileSchemaErr = err
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.UseRegexpEngine(compileECMARegexp)
		if err := compiler.AddResource(schemaResource, instance); err != nil {
			compileSchemaErr = err
			return
		}
		compiledSchema, compileSchemaErr = compiler.Compile(schemaResource)
	})
	return compiledSchema, compileSchemaErr
}

type ecmaRegexp regexp2.Regexp

func (expression *ecmaRegexp) MatchString(value string) bool {
	matched, err := (*regexp2.Regexp)(expression).MatchString(value)
	return err == nil && matched
}

func (expression *ecmaRegexp) String() string {
	return (*regexp2.Regexp)(expression).String()
}

func compileECMARegexp(pattern string) (jsonschema.Regexp, error) {
	expression, err := regexp2.Compile(pattern, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return (*ecmaRegexp)(expression), nil
}

func decodeJSON(raw []byte) (any, error) {
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
	return value, nil
}

func validateConsumerIdentity(document acceptedDocument, expectedComponent, expectedVersion string) error {
	if document.Identity.Component != expectedComponent {
		return fmt.Errorf("identity component %q does not match publication directory %q", document.Identity.Component, expectedComponent)
	}
	if document.Identity.VersionScope != expectedVersion {
		return fmt.Errorf("identity version_scope %q does not match resolved version %q", document.Identity.VersionScope, expectedVersion)
	}
	return nil
}

func mapDocument(document acceptedDocument) (*types.ComponentDoc, error) {
	view := document.RenderingView
	doc := &types.ComponentDoc{
		Name: view.Component,
		Metadata: map[string]string{
			"Repository": view.Metadata.Repository, "Version": view.Metadata.Version,
			"Distribution": view.Metadata.Distribution, "Languages": view.Metadata.Languages,
			"Deployment Type": view.Metadata.DeploymentType, "Generated By": view.Metadata.GeneratedBy,
		},
		Repository: view.Metadata.Repository, Version: view.Metadata.Version,
		Languages: view.Metadata.Languages, DeployType: view.Metadata.DeploymentType,
		CommitSHA: document.Identity.SourceRevision, AnalyzerVersion: document.Producers.AnalyzerVersion,
		DocumentSchemaVersion: document.SchemaVersion, SourceComponent: document.Identity.SourceComponent,
		VersionScope: document.Identity.VersionScope, IntegrationStatus: document.Identity.IntegrationStatus,
		Aliases: append([]string(nil), document.Identity.Aliases...), BundleFingerprint: document.AnalyzerInput.BundleFingerprint,
	}
	doc.Purpose = deterministicShortPurpose(view)
	doc.PurposeFull = deterministicDetailedPurpose(view)

	for _, item := range view.ArchitectureComponents {
		row := legacyRow(item.Component, item.Type, item.Purpose)
		doc.Components = append(doc.Components, types.ArchComponent{Name: row[0], Type: row[1], Purpose: row[2]})
	}
	for _, item := range view.CRDs {
		// Preserve the current Markdown reader's five-column compatibility
		// projection. In the current six-column renderer, its Purpose field sees
		// the API Role column.
		row := legacyRow(item.Group, item.Version, item.Kind, item.Scope, item.APIRole, item.Purpose)
		doc.CRDs = append(doc.CRDs, types.CRD{Group: row[0], Version: row[1], Kind: row[2], Scope: row[3], Purpose: row[4]})
	}
	for _, item := range view.HTTPEndpoints {
		// These assignments intentionally mirror the existing positional
		// Markdown adapter, including fields added to the renderer after it.
		row := legacyRow(item.Path, item.Method, item.Port, item.Protocol, item.Transport, item.Encryption, item.Auth, item.Owner, item.Purpose)
		doc.Endpoints = append(doc.Endpoints, types.Endpoint{Path: row[0], Method: row[1], Port: row[2], Protocol: row[3], Encryption: row[4], Auth: row[5], Purpose: row[6]})
	}
	for _, item := range view.GRPCServices {
		row := legacyRow(item.Service, item.Port, item.Protocol, item.Transport, item.Encryption, item.Auth, item.Owner, item.Purpose)
		doc.GRPCServices = append(doc.GRPCServices, types.GRPCService{Service: row[0], Port: row[1], Protocol: row[2], Encryption: row[3], Auth: row[4], Purpose: row[5]})
	}
	for _, item := range view.ExternalDependencies {
		row := legacyRow(item.Component, item.Version, item.Required, item.Role, item.Purpose)
		doc.ExternalDeps = append(doc.ExternalDeps, types.Dependency{Component: row[0], Version: row[1], Required: row[2], Purpose: row[3]})
	}
	for _, item := range view.InternalDependencies {
		row := legacyRow(item.Component, item.InteractionType, item.Role, item.Purpose)
		doc.InternalDeps = append(doc.InternalDeps, types.Dependency{Component: row[0], InteractionType: row[1], Purpose: row[2]})
	}
	for _, item := range view.Services {
		row := legacyRow(item.Name, item.Type, item.Port, item.TargetPort, item.Protocol, item.Encryption, item.Auth, item.Exposure)
		doc.Services = append(doc.Services, types.Service{Name: row[0], Type: row[1], Port: row[2], TargetPort: row[3], Protocol: row[4], Encryption: row[5], Auth: row[6], Exposure: row[7]})
	}
	for _, item := range view.Ingress {
		row := legacyRow(item.Name, item.Type, item.Hosts, item.Port, item.Protocol, item.Encryption, item.TLSMode, item.Exposure)
		doc.Ingresses = append(doc.Ingresses, types.Ingress{Component: row[0], Type: row[1], Hosts: row[2], Port: row[3], Protocol: row[4], Encryption: row[5], TLSMode: row[6], Exposure: row[7]})
	}
	for _, item := range view.Egress {
		row := legacyRow(item.Destination, item.Port, item.Protocol, item.Encryption, item.Auth, item.Purpose)
		doc.Egresses = append(doc.Egresses, types.Egress{Destination: row[0], Port: row[1], Protocol: row[2], Encryption: row[3], Auth: row[4], Purpose: row[5]})
	}
	for _, item := range view.ClusterRoles {
		row := legacyRow(item.Name, item.APIGroup, item.Resources, item.NonResourceURLs, item.Verbs)
		doc.RBACRoles = append(doc.RBACRoles, types.RBACRole{RoleName: row[0], APIGroup: row[1], Resources: row[2], NonResourceURLs: row[3], Verbs: row[4]})
	}

	for _, item := range document.Facts {
		if len(item.Evidence) == 0 {
			doc.ProvenanceGaps = append(doc.ProvenanceGaps, types.ProvenanceGap{
				FactID: item.ID, FactType: item.Type,
				Reason: "accepted fact has no source evidence reference",
			})
		}
		for _, ref := range item.Evidence {
			doc.SourceCitations = append(doc.SourceCitations, citation(ref, item.ID, item.Type, "", "", item.Authority))
		}
		if item.Authority.ClaimClass == "planned" || item.Authority.ClaimClass == "support" {
			continue
		}
		if err := mapTypedFact(doc, item); err != nil {
			return nil, err
		}
	}
	for _, item := range document.Sections {
		for _, ref := range item.Evidence {
			doc.SourceCitations = append(doc.SourceCitations, citation(ref, "", "", item.ID, "", item.Authority))
		}
		for _, content := range item.Blocks {
			for _, row := range content.Rows {
				for _, ref := range row.Evidence {
					doc.SourceCitations = append(doc.SourceCitations, citation(ref, "", "", item.ID, content.TableID, item.Authority))
				}
			}
		}
	}
	for _, item := range document.Dispositions {
		mapped := types.ProposalDisposition{
			PatchID: item.PatchID, ProposalFingerprint: item.ProposalFingerprint, BundleFingerprint: item.BundleFingerprint,
			OperationID: item.OperationID, Action: item.Action, FactType: item.FactType, OriginKind: item.OriginKind,
			OriginID: item.OriginID, ClaimClass: item.ClaimClass, Status: item.Status, TargetFactID: item.TargetFactID,
			ResultingFactID: item.ResultingFactID, ProposalReason: item.ProposalReason, PolicyID: item.PolicyID,
			AuthorizedBy: item.AuthorizedBy, AllowedFactTypes: append([]string(nil), item.AllowedFactTypes...),
			DecisionID: item.DecisionID, DecidedBy: item.DecidedBy, DecisionReason: item.DecisionReason,
		}
		for _, ref := range item.Evidence {
			mapped.Evidence = append(mapped.Evidence, citation(ref, "", item.FactType, "", "", authority{Kind: item.OriginKind, Origin: item.OriginID, ClaimClass: item.ClaimClass}))
		}
		doc.ProposalDispositions = append(doc.ProposalDispositions, mapped)
	}
	return doc, nil
}

// legacyRow reproduces only the current Markdown table reader's cell boundary
// behavior. This keeps typed query results stable for renderer-escaped pipes
// without rendering a document or accepting Markdown as fact authority.
func legacyRow(values ...string) []string {
	for index, value := range values {
		value = strings.ReplaceAll(value, "\r", " ")
		value = strings.ReplaceAll(value, "\n", " ")
		values[index] = strings.TrimSpace(strings.ReplaceAll(value, "|", "\\|"))
	}
	line := strings.TrimPrefix(strings.TrimSuffix("| "+strings.Join(values, " | ")+" |", "|"), "|")
	parts := strings.Split(line, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func citation(ref evidence, factID, factType, sectionID, tableID string, origin authority) types.SourceCitation {
	return types.SourceCitation{FactID: factID, FactType: factType, SectionID: sectionID, TableID: tableID, Path: ref.Path, StartLine: ref.StartLine, EndLine: ref.EndLine, Revision: ref.Revision, OriginKind: origin.Kind, OriginID: origin.Origin, ClaimClass: origin.ClaimClass}
}

func mapTypedFact(doc *types.ComponentDoc, item fact) error {
	decode := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(item.Value))
		decoder.DisallowUnknownFields()
		decoder.UseNumber()
		if err := decoder.Decode(target); err != nil {
			return fmt.Errorf("fact %q (%s): %w", item.ID, item.Type, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return fmt.Errorf("fact %q (%s) has trailing JSON", item.ID, item.Type)
		}
		return nil
	}
	switch item.Type {
	case "controller_watch":
		var value struct {
			Type        string `json:"type"`
			GVK         string `json:"gvk"`
			Controller  string `json:"controller"`
			Source      string `json:"source"`
			Conditional bool   `json:"conditional,omitempty"`
			Operations  []any  `json:"resource_ops"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		doc.ControllerWatches = append(doc.ControllerWatches, types.ControllerWatch{Type: value.Type, GVK: value.GVK, Controller: value.Controller, Source: value.Source})
	case "webhook":
		var value struct {
			Name          string                `json:"name"`
			Type          string                `json:"type"`
			ServiceRef    string                `json:"service_ref"`
			Path          string                `json:"path"`
			FailurePolicy string                `json:"failure_policy"`
			Purpose       string                `json:"purpose"`
			Port          any                   `json:"port"`
			Rules         []types.WebhookRule   `json:"rules"`
			Sources       []types.WebhookSource `json:"sources"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		port, err := integer(value.Port)
		if err != nil {
			return fmt.Errorf("fact %q webhook port: %w", item.ID, err)
		}
		doc.Webhooks = append(doc.Webhooks, types.Webhook{Name: value.Name, Type: value.Type, ServiceRef: value.ServiceRef, Path: value.Path, Port: port, FailurePolicy: value.FailurePolicy, Rules: value.Rules, Sources: value.Sources, Purpose: value.Purpose})
	case "external_webhook":
		var value types.WebhookRef
		if err := decode(&value); err != nil {
			return err
		}
		doc.ExternalWebhooks = append(doc.ExternalWebhooks, value)
	case "dockerfile":
		var value struct {
			Path      string   `json:"path"`
			BaseImage string   `json:"base_image"`
			User      string   `json:"user"`
			Issues    []string `json:"issues"`
		}
		if err := decode(&value); err != nil {
			return err
		}
		doc.Dockerfiles = append(doc.Dockerfiles, types.Dockerfile{Path: value.Path, BaseImage: value.BaseImage, User: value.User, Issues: value.Issues})
	case "cross_cutting_evidence":
		var value types.CrossCuttingEvidence
		if err := decode(&value); err != nil {
			return err
		}
		if doc.CrossCuttingEvidence == nil {
			doc.CrossCuttingEvidence = make(map[string][]types.CrossCuttingEvidence)
		}
		doc.CrossCuttingEvidence[item.Key] = append(doc.CrossCuttingEvidence[item.Key], value)
	}
	return nil
}

func integer(value any) (int, error) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil || strconv.IntSize == 32 && (parsed > math.MaxInt32 || parsed < math.MinInt32) {
			return 0, fmt.Errorf("non-integral or out-of-range value %q", typed)
		}
		return int(parsed), nil
	case float64:
		if math.Trunc(typed) != typed || typed > float64(maxInt()) || typed < float64(-maxInt()-1) {
			return 0, fmt.Errorf("non-integral or out-of-range value %v", typed)
		}
		return int(typed), nil
	case string:
		if typed == "" {
			return 0, nil
		}
		return strconv.Atoi(typed)
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported value %T", value)
	}
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func deterministicShortPurpose(document renderingView) string {
	component := proseFallback(document.Component, "This component")
	interfaceCount := len(document.HTTPEndpoints) + len(document.GRPCServices) + len(document.CRDs) + len(document.ServingRuntimes)
	return fmt.Sprintf("Source-backed analysis represents %s as %s with %s, %s, and %s.", component, proseFallback(document.Metadata.DeploymentType, "an architecture component"), countPhrase(len(document.ArchitectureComponents), "runtime component"), countPhrase(interfaceCount, "API identity"), countPhrase(len(document.IntegrationPoints), "integration point")) + sourceCitation(document, "Architecture Components", "APIs Exposed", "Dependencies")
}

func deterministicDetailedPurpose(document renderingView) string {
	component := proseValue(document.Component)
	if component == "" {
		component = "This component"
	}
	parts := []string{fmt.Sprintf("%s is represented by %s in the extracted architecture evidence.", component, countPhrase(len(document.ArchitectureComponents), "architecture component"))}
	if names := architectureComponentSummaries(document); len(names) > 0 {
		parts = append(parts, "The principal extracted components are "+joinedList(names)+".")
	}
	interfaceCount := len(document.HTTPEndpoints) + len(document.GRPCServices) + len(document.CRDs) + len(document.ServingRuntimes)
	if interfaceCount > 0 {
		parts = append(parts, fmt.Sprintf("Its documented interface surface contains %s, including %s.", countPhrase(interfaceCount, "API identity"), interfaceSummary(document)))
	}
	if len(document.IntegrationPoints)+len(document.InternalDependencies) > 0 {
		parts = append(parts, fmt.Sprintf("The extracted dependency view records %s and %s.", countPhrase(len(document.InternalDependencies), "internal platform dependency"), countPhrase(len(document.IntegrationPoints), "integration point")))
	}
	parts = append(parts, "This description is limited to typed, source-backed analyzer facts."+sourceCitation(document, "Architecture Components", "APIs Exposed", "Dependencies"))
	return strings.Join(parts, " ")
}

func architectureComponentSummaries(document renderingView) []string {
	values := make([]string, 0, min(len(document.ArchitectureComponents), synthesisListLimit))
	for _, row := range document.ArchitectureComponents {
		value := proseValue(row.Component)
		var details []string
		if row.Type != "" {
			details = append(details, proseValue(row.Type))
		}
		if row.Purpose != "" {
			details = append(details, proseValue(row.Purpose))
		}
		if len(details) > 0 {
			value += " (" + strings.Join(details, "; ") + ")"
		}
		if value != "" {
			values = append(values, value)
		}
		if len(values) == synthesisListLimit {
			break
		}
	}
	if len(document.ArchitectureComponents) > len(values) {
		values = append(values, countPhrase(len(document.ArchitectureComponents)-len(values), "additional component")+" listed in the table")
	}
	return values
}

func interfaceSummary(document renderingView) string {
	var parts []string
	if len(document.HTTPEndpoints) > 0 {
		parts = append(parts, countPhrase(len(document.HTTPEndpoints), "HTTP endpoint"))
	}
	if len(document.GRPCServices) > 0 {
		parts = append(parts, countPhrase(len(document.GRPCServices), "gRPC service"))
	}
	if len(document.CRDs) > 0 {
		parts = append(parts, countPhrase(len(document.CRDs), "custom resource identity"))
	}
	if len(document.ServingRuntimes) > 0 {
		parts = append(parts, countPhrase(len(document.ServingRuntimes), "serving runtime definition"))
	}
	return joinedList(parts)
}

func sourceCitation(document renderingView, sections ...string) string {
	wanted := make(map[string]bool, len(sections))
	for _, section := range sections {
		wanted[section] = true
	}
	refs := make([]string, 0, synthesisListLimit)
	for _, source := range document.Sources {
		matched := false
		for _, section := range strings.Split(source.Sections, ",") {
			if wanted[strings.TrimSpace(section)] {
				matched = true
				break
			}
		}
		if !matched || strings.TrimSpace(source.File) == "" {
			continue
		}
		ref := proseValue(source.File)
		if strings.TrimSpace(source.Lines) != "" && strings.TrimSpace(source.Lines) != "Unknown" {
			ref += ":" + collapseLineRanges(source.Lines)
		}
		refs = append(refs, ref)
		if len(refs) == synthesisListLimit {
			break
		}
	}
	if len(refs) == 0 {
		return " [source: no section-specific file recorded]"
	}
	return " [source: " + strings.Join(refs, ", ") + "]"
}

func countPhrase(count int, singular string) string {
	if count == 1 {
		return "1 " + singular
	}
	plural := singular + "s"
	if strings.HasSuffix(singular, "y") {
		plural = strings.TrimSuffix(singular, "y") + "ies"
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func joinedList(values []string) string {
	values = nonEmpty(values)
	switch len(values) {
	case 0:
		return "none recorded"
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = proseValue(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func proseFallback(value, fallback string) string {
	if value = proseValue(value); value != "" {
		return value
	}
	return fallback
}

func proseValue(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "/")
	return strings.Join(strings.Fields(value), " ")
}

func collapseLineRanges(lines string) string {
	parts := strings.Split(lines, ",")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		if number, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			numbers = append(numbers, number)
		}
	}
	if len(numbers) == 0 {
		return strings.TrimSpace(lines)
	}
	sort.Ints(numbers)
	var ranges []string
	start, end := numbers[0], numbers[0]
	for _, number := range numbers[1:] {
		if number == end+1 {
			end = number
			continue
		}
		ranges = append(ranges, lineRange(start, end))
		start, end = number, number
	}
	ranges = append(ranges, lineRange(start, end))
	return strings.Join(ranges, ", ")
}

func lineRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}
