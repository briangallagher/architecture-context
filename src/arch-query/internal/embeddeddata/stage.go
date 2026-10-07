// Package embeddeddata stages the bounded architecture subset compiled into
// arch-query release binaries.
package embeddeddata

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	analyzerdocument "github.com/jctanner/arch-analyzer/pkg/document"
	"github.com/jctanner/arch-query/internal/documentdata"
)

var acceptedArtifactNames = map[string]bool{
	"analyzer.json":  true,
	"document.json":  true,
	"synthesis.json": true,
}

var nonComponentDirectories = map[string]bool{
	"diagrams": true, "logs": true, "metadata": true, "overlays": true,
	"prompts": true, "run-metadata": true, "runs": true,
}

// Stage copies queryable legacy and accepted architecture artifacts. The
// destination must already exist; callers own cleanup so tests never touch the
// repository's historical architecture tree.
func Stage(source, destination, overlays string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read architecture source: %w", err)
	}
	symlinks := make(map[string]string)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(filepath.Join(source, entry.Name()))
			if readErr != nil {
				return fmt.Errorf("read version alias %s: %w", entry.Name(), readErr)
			}
			symlinks[entry.Name()] = target
			continue
		}
		if !entry.IsDir() || entry.Name() == "diagrams" || entry.Name() == "overlays" {
			continue
		}
		if err := stageVersion(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return fmt.Errorf("stage version %s: %w", entry.Name(), err)
		}
	}
	if err := writeSymlinks(filepath.Join(destination, "symlinks.json"), symlinks); err != nil {
		return err
	}
	if overlays != "" {
		if info, statErr := os.Stat(overlays); statErr == nil && info.IsDir() {
			if err := copyTree(overlays, filepath.Join(destination, "overlays")); err != nil {
				return fmt.Errorf("stage overlays: %w", err)
			}
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("inspect overlays: %w", statErr)
		}
	}
	return nil
}

func stageVersion(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		if !entry.IsDir() {
			if strings.HasSuffix(entry.Name(), ".md") || strings.HasSuffix(entry.Name(), ".json") {
				if err := copyFile(from, to); err != nil {
					return err
				}
			}
			continue
		}
		if entry.Name() == "contracts" {
			if err := copyTree(from, to); err != nil {
				return err
			}
			continue
		}
		if nonComponentDirectories[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if err := validateComponentSnapshot(source, from, entry.Name()); err != nil {
			return err
		}
		if err := stageComponent(from, to); err != nil {
			return err
		}
	}
	return nil
}

func validateComponentSnapshot(versionSource, componentSource, component string) error {
	documentPath := filepath.Join(componentSource, "document.json")
	document, err := os.ReadFile(documentPath)
	if os.IsNotExist(err) {
		entries, readErr := os.ReadDir(componentSource)
		if readErr != nil {
			return fmt.Errorf("inspect component %s publication state: %w", component, readErr)
		}
		for _, entry := range entries {
			name := entry.Name()
			if acceptedArtifactNames[name] || strings.HasPrefix(name, ".analyzer.json.") ||
				strings.HasPrefix(name, ".synthesis.json.") || strings.HasPrefix(name, ".document.json.") {
				return fmt.Errorf("component %s has accepted artifact %s without document.json", component, name)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read accepted document for %s: %w", component, err)
	}
	if err := analyzerdocument.ValidateJSON(document); err != nil {
		return fmt.Errorf("validate accepted document for %s: %w", component, err)
	}
	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(document, &header); err != nil {
		return fmt.Errorf("decode accepted document version for %s: %w", component, err)
	}
	if header.SchemaVersion != "1.1.0" {
		return nil
	}
	analyzer, err := os.ReadFile(filepath.Join(componentSource, "analyzer.json"))
	if err != nil {
		return fmt.Errorf("read published analyzer for %s: %w", component, err)
	}
	synthesis, err := os.ReadFile(filepath.Join(componentSource, "synthesis.json"))
	if err != nil {
		return fmt.Errorf("read published synthesis for %s: %w", component, err)
	}
	if err := documentdata.ValidateSynthesisJSON(synthesis); err != nil {
		return fmt.Errorf("validate published synthesis for %s: %w", component, err)
	}
	markdown, err := os.ReadFile(filepath.Join(versionSource, component+".md"))
	if err != nil {
		return fmt.Errorf("read published Markdown for %s: %w", component, err)
	}
	if err := analyzerdocument.ValidatePublicationJSON(document, analyzer, synthesis, markdown); err != nil {
		return fmt.Errorf("validate complete published snapshot for %s: %w", component, err)
	}
	return nil
}

func stageComponent(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		switch {
		case !entry.IsDir() && acceptedArtifactNames[entry.Name()]:
			if err := copyFile(from, to); err != nil {
				return err
			}
		case entry.IsDir() && entry.Name() == "diagrams":
			if err := copyTree(from, to); err != nil {
				return err
			}
		case entry.IsDir() && entry.Name() == ".analyzer":
			legacy := filepath.Join(from, "component-architecture.json")
			if info, statErr := os.Stat(legacy); statErr == nil && !info.IsDir() {
				if err := copyFile(legacy, filepath.Join(to, "component-architecture.json")); err != nil {
					return err
				}
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return statErr
			}
		}
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func writeSymlinks(path string, aliases map[string]string) error {
	keys := make([]string, 0, len(aliases))
	for key := range aliases {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, key := range keys {
		ordered[key] = aliases[key]
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
