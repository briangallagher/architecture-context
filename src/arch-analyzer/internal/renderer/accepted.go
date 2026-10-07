package renderer

import (
	"fmt"
	"io"

	"github.com/jctanner/arch-analyzer/internal/structured"
)

// AcceptedMarkdown is the structured route into the existing renderer. The
// accepted document is its only content input; no analyzer file, source tree,
// candidate Markdown, or model response is read here.
func AcceptedMarkdown(writer io.Writer, document structured.Document) error {
	if err := structured.Validate(document); err != nil {
		return fmt.Errorf("validate accepted document: %w", err)
	}
	view := document.RenderingView
	view.StructuredSections = append(view.StructuredSections[:0], document.Sections...)
	return Markdown(writer, view)
}
