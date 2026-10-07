package extractor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jctanner/arch-analyzer/internal/model"
)

// Entrypoints and integration points are accumulated from independent
// manifest and language producers. Their order is navigation order, not source
// semantics, so canonicalize the complete emitted rows after all producers have
// contributed. Nested values whose ordering is meaningful are left untouched.
func sortEntrypoints(records []model.Entrypoint) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		return strings.Join([]string{
			left.Source, left.Name, left.Type, left.Runtime, left.Command, left.WorkloadRef,
		}, "\x00") < strings.Join([]string{
			right.Source, right.Name, right.Type, right.Runtime, right.Command, right.WorkloadRef,
		}, "\x00")
	})
}

func sortIntegrationPoints(records []model.IntegrationFact) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		return strings.Join([]string{
			left.Source,
			left.Component,
			left.InteractionType,
			left.Role,
			fmt.Sprintf("%T:%v", left.Port, left.Port),
			left.Protocol,
			left.Encryption,
			left.Purpose,
		}, "\x00") < strings.Join([]string{
			right.Source,
			right.Component,
			right.InteractionType,
			right.Role,
			fmt.Sprintf("%T:%v", right.Port, right.Port),
			right.Protocol,
			right.Encryption,
			right.Purpose,
		}, "\x00")
	})
}
