package gosource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/model"
)

func TestBehavioralEvidenceMatchesPinnedRhodsOperatorShapes(t *testing.T) {
	result, err := Extract("testdata/behavioral-surfaces")
	if err != nil {
		t.Fatal(err)
	}

	metrics := behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind)
	if len(metrics) != 2 {
		t.Fatalf("metrics behavior = %#v, want observed and unresolved records", metrics)
	}
	observedMetrics := observedBehavior(metrics)
	if observedMetrics == nil {
		t.Fatalf("metrics behavior = %#v, want observed conditional control", metrics)
	}
	if observedMetrics.ServingSurface != "controller-runtime metrics serving surface" ||
		observedMetrics.ConfigurationBranch != "oconfig.MetricsSecure is true" ||
		observedMetrics.EnforcementProvider != "filters.WithAuthenticationAndAuthorization" ||
		observedMetrics.Source != "cmd/main.go:16-25" {
		t.Fatalf("metrics behavior = %#v, want pinned conditional serving semantics", observedMetrics)
	}
	if unresolvedBehavior(metrics) == nil || len(unresolvedBehavior(metrics).Limitations) == 0 {
		t.Fatalf("metrics behavior = %#v, want explicit unsupported-wrapper limitation", metrics)
	}

	watches := behavioralByKind(result.BehavioralEvidence, namedWatchKind)
	var names []string
	for _, watch := range watches {
		if watch.Status != behaviorObserved || watch.Identity != "internal/controller/services/auth.ServiceHandler" {
			continue
		}
		names = append(names, watch.LiteralValues...)
		if watch.WatchedGVK != "/v1/Namespace" ||
			watch.EventTarget != "services.platform.opendatahub.io/v1alpha1/Auth/auth" ||
			!strings.HasPrefix(watch.Source, "internal/controller/services/auth/controller.go:") {
			t.Errorf("watch behavior = %#v, want source-backed Namespace routing", watch)
		}
	}
	names = uniqueSortedStrings(names)
	if strings.Join(names, ",") != "kuadrant-system,models-as-a-service" {
		t.Fatalf("literal watch names = %#v, want MaaS and Kuadrant namespaces", names)
	}

	statuses := map[string]string{}
	for _, watch := range watches {
		statuses[watch.Identity] = watch.Status
	}
	if statuses["internal/controller/collision/auth.ServiceHandler"] != behaviorUnresolved ||
		statuses["internal/controller/services/auth.ServiceHandler"] != behaviorObserved {
		t.Fatalf("watch identities/statuses = %#v, want package-qualified collision separation", statuses)
	}
	if statuses["internal/controller/dynamic/auth.ServiceHandler"] != behaviorUnresolved {
		t.Fatalf("watch identities/statuses = %#v, want same-named unsupported wrapper unresolved", statuses)
	}
	for _, watch := range watches {
		if watch.Identity == "internal/controller/dynamic/auth.ServiceHandler" && len(watch.LiteralValues) != 0 {
			t.Fatalf("unsupported same-named predicate became literal evidence: %#v", watch)
		}
	}
}

func TestConditionalMetricsBehaviorDoesNotGeneralizeFromMethodName(t *testing.T) {
	root := writeSecurityRepository(t, `package main
import (
  ctrl "sigs.k8s.io/controller-runtime"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
  misleading "example.com/not-controller-runtime/filters"
)
func run(secure bool) (any, error) {
  return ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
    opts := ctrlmetrics.Options{SecureServing: secure}
    if secure { opts.FilterProvider = misleading.WithAuthenticationAndAuthorization }
    return opts
  }()})
}
`)
	result, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	records := behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind)
	if len(records) != 1 || records[0].Status != behaviorUnresolved || records[0].EnforcementProvider != "" {
		t.Fatalf("metrics behavior = %#v, want same-named provider unresolved", records)
	}
}

func TestConditionalMetricsBehaviorRequiresMatchingSecureServingBranch(t *testing.T) {
	root := writeSecurityRepository(t, `package main
import (
  ctrl "sigs.k8s.io/controller-runtime"
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)
func run(secure, enabled bool) (any, error) {
  return ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
    opts := ctrlmetrics.Options{SecureServing: secure}
    if enabled { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
    return opts
  }()})
}
`)
	result, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	records := behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind)
	if len(records) != 1 || records[0].Status != behaviorUnresolved || records[0].ConfigurationBranch != "" {
		t.Fatalf("metrics behavior = %#v, want mismatched condition unresolved", records)
	}
}

func TestConditionalMetricsBehaviorRequiresManagerAttachment(t *testing.T) {
	tests := map[string]string{
		"dead standalone options": `package main
import (
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)
func dead(secure bool) {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  _ = opts
}`,
		"foreign manager options": `package main
import (
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)
type Options struct { Metrics ctrlmetrics.Options }
func NewManager(_ any, _ Options) (any, error) { return nil, nil }
func foreign(secure bool) (any, error) {
  return NewManager(nil, Options{Metrics: func() ctrlmetrics.Options {
    opts := ctrlmetrics.Options{SecureServing: secure}
    if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
    return opts
  }()})
}`,
		"foreign metrics field passed to manager": `package main
import (
  ctrl "sigs.k8s.io/controller-runtime"
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
)
type foreignMetrics struct { SecureServing bool; FilterProvider any }
func foreign(secure bool) (any, error) {
  return ctrl.NewManager(nil, ctrl.Options{Metrics: func() foreignMetrics {
    opts := foreignMetrics{SecureServing: secure}
    if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
    return opts
  }()})
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := Extract(writeSecurityRepository(t, source))
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind) {
				if record.Status == behaviorObserved {
					t.Fatalf("metrics behavior = %#v, want disconnected/foreign options non-observed", result.BehavioralEvidence)
				}
			}
		})
	}
}

func TestConditionalMetricsBehaviorRequiresStaticLexicalBinding(t *testing.T) {
	tests := map[string]string{
		"dynamic expression": `func dynamicSecure() bool { return true }
func run() { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: dynamicSecure()}
  if dynamicSecure() { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  return opts
}()}) }`,
		"reassigned condition": `func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  secure = !secure
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  return opts
}()}) }`,
		"if initializer shadow": `func run(secure, other bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure := other; secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  return opts
}()}) }`,
		"shadowed options": `type unrelated struct { FilterProvider any }
func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  func() {
    opts := unrelated{}
    if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  }()
  return opts
}()}) }`,
		"if block shadow": `type unrelated struct { FilterProvider any }
func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure {
    opts := unrelated{}
    opts.FilterProvider = filters.WithAuthenticationAndAuthorization
  }
  return opts
}()}) }`,
		"filter reset before return": `func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  opts.FilterProvider = nil
  return opts
}()}) }`,
		"options rebound before return": `func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  opts = ctrlmetrics.Options{SecureServing: secure}
  return opts
}()}) }`,
		"secure serving changed before return": `func run(secure bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  opts.SecureServing = false
  return opts
}()}) }`,
		"nested alternate return before filter": `func run(secure, bypass bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if bypass { return ctrlmetrics.Options{SecureServing: secure} }
  if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
  return opts
}()}) }`,
		"nested alternate return inside secure branch": `func run(secure, bypass bool) { _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
  opts := ctrlmetrics.Options{SecureServing: secure}
  if secure {
    if bypass { return opts }
    opts.FilterProvider = filters.WithAuthenticationAndAuthorization
  }
  return opts
}()}) }`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := fmt.Sprintf(`package main
import (
  ctrl "sigs.k8s.io/controller-runtime"
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)
%s
`, body)
			result, err := Extract(writeSecurityRepository(t, source))
			if err != nil {
				t.Fatal(err)
			}
			records := behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind)
			if len(records) != 1 || records[0].Status != behaviorUnresolved {
				t.Fatalf("metrics behavior = %#v, want one dynamic/reassigned/shadowed unresolved record", records)
			}
		})
	}
}

func TestConditionalMetricsBehaviorAllowsUnrelatedCertificateWritesBeforeReturn(t *testing.T) {
	root := writeSecurityRepository(t, `package main
import (
  ctrl "sigs.k8s.io/controller-runtime"
  filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
  ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)
func run(secure bool, certDir string) {
  _, _ = ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
    opts := ctrlmetrics.Options{SecureServing: secure}
    if secure { opts.FilterProvider = filters.WithAuthenticationAndAuthorization }
    opts.CertDir = certDir
    opts.CertName = "tls.crt"
    opts.KeyName = "tls.key"
    return opts
  }()})
}
`)
	result, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	records := behavioralByKind(result.BehavioralEvidence, conditionalMetricsKind)
	if len(records) != 1 || records[0].Status != behaviorObserved {
		t.Fatalf("metrics behavior = %#v, want certificate writes to preserve observed filter final state", records)
	}
}

func TestNamedWatchBehaviorRejectsUnsupportedPredicateComposition(t *testing.T) {
	root := writeSecurityRepository(t, `package main
import (
  corev1 "k8s.io/api/core/v1"
  predicate "sigs.k8s.io/controller-runtime/pkg/predicate"
  resources "example.com/security/pkg/controller/predicates/resources"
  reconciler "example.com/security/pkg/controller/reconciler"
)
type Handler struct{}
func (h *Handler) Setup(mgr any) {
  reconciler.ReconcilerFor(mgr, &corev1.Namespace{}).Watches(
    &corev1.Namespace{},
    reconciler.WithPredicates(predicate.Or(resources.CreatedOrUpdatedOrDeletedNamed("literal-but-wrapped"))),
  )
}
`)
	result, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	records := behavioralByKind(result.BehavioralEvidence, namedWatchKind)
	if len(records) != 1 || records[0].Status != behaviorUnresolved || len(records[0].LiteralValues) != 0 {
		t.Fatalf("watch behavior = %#v, want unsupported composition unresolved without literal claim", records)
	}
}

func TestNamedWatchBehaviorRetainsLiteralButMarksDynamicHandlerTargetUnresolved(t *testing.T) {
	root := writeSecurityRepository(t, `package main
import (
  corev1 "k8s.io/api/core/v1"
  handlers "example.com/security/pkg/controller/handlers"
  resources "example.com/security/pkg/controller/predicates/resources"
  reconciler "example.com/security/pkg/controller/reconciler"
)
type Handler struct{}
func targetName() string { return "dynamic" }
func (h *Handler) Setup(mgr any) {
  reconciler.ReconcilerFor(mgr, &corev1.Namespace{}).Watches(
    &corev1.Namespace{},
    reconciler.WithEventHandler(handlers.ToNamed(targetName())),
    reconciler.WithPredicates(resources.CreatedOrUpdatedOrDeletedNamed("literal-name")),
  )
}
`)
	result, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	records := behavioralByKind(result.BehavioralEvidence, namedWatchKind)
	if len(records) != 1 || records[0].Status != behaviorUnresolved ||
		strings.Join(records[0].LiteralValues, ",") != "literal-name" || records[0].EventTarget != "" {
		t.Fatalf("watch behavior = %#v, want literal predicate with unresolved dynamic target", records)
	}
}

func TestNamedWatchBehaviorRequiresExactModuleHelpersAndDirectHandlerComposition(t *testing.T) {
	tests := map[string]string{
		"foreign matching suffixes": `
  foreignhandlers "foreign.example/pkg/controller/handlers"
  foreignresources "foreign.example/pkg/controller/predicates/resources"
  foreignreconciler "foreign.example/pkg/controller/reconciler"`,
		"arbitrary event wrapper": `
  handlers "example.com/security/pkg/controller/handlers"
  resources "example.com/security/pkg/controller/predicates/resources"
  reconciler "example.com/security/pkg/controller/reconciler"
  foreign "foreign.example/wrappers"`,
		"mixed handler helper": `
  handlers "example.com/security/pkg/controller/handlers"
  resources "example.com/security/pkg/controller/predicates/resources"
  reconciler "example.com/security/pkg/controller/reconciler"
  foreign "foreign.example/pkg/controller/reconciler"`,
		"extra nested ToNamed": `
  handlers "example.com/security/pkg/controller/handlers"
  resources "example.com/security/pkg/controller/predicates/resources"
  reconciler "example.com/security/pkg/controller/reconciler"
  foreign "foreign.example/wrappers"`,
	}
	for name, imports := range tests {
		t.Run(name, func(t *testing.T) {
			reconcilerName, resourcesName := "reconciler", "resources"
			var handlerExpression string
			if name == "foreign matching suffixes" {
				reconcilerName, resourcesName = "foreignreconciler", "foreignresources"
				handlerExpression = `foreignreconciler.WithEventHandler(foreignhandlers.ToNamed("target"))`
			} else if name == "arbitrary event wrapper" {
				handlerExpression = `reconciler.WithEventHandler(foreign.Wrap(handlers.ToNamed("target")))`
			} else if name == "extra nested ToNamed" {
				handlerExpression = `reconciler.WithEventHandler(handlers.ToNamed("target")),
    foreign.Wrap(handlers.ToNamed("other"))`
			} else {
				handlerExpression = `reconciler.WithEventHandler(handlers.ToNamed("target")),
    foreign.WithEventHandler(handlers.ToNamed("other"))`
			}
			source := fmt.Sprintf(`package main
import (
  corev1 "k8s.io/api/core/v1"%s
)
type Handler struct{}
func (h *Handler) Setup(mgr any) {
  %s.ReconcilerFor(mgr, &corev1.Namespace{}).Watches(
    &corev1.Namespace{},
    %s,
    %s.WithPredicates(%s.CreatedOrUpdatedOrDeletedNamed("literal")),
  )
}
`, imports, reconcilerName, handlerExpression, reconcilerName, resourcesName)
			result, err := Extract(writeSecurityRepository(t, source))
			if err != nil {
				t.Fatal(err)
			}
			records := behavioralByKind(result.BehavioralEvidence, namedWatchKind)
			if len(records) != 1 || records[0].Status != behaviorUnresolved || records[0].EventTarget != "" {
				t.Fatalf("watch behavior = %#v, want foreign/wrapped helper unresolved", records)
			}
		})
	}
}

func behavioralByKind(records []model.BehavioralEvidence, kind string) []model.BehavioralEvidence {
	var result []model.BehavioralEvidence
	for _, record := range records {
		if record.Kind == kind {
			result = append(result, record)
		}
	}
	return result
}

func observedBehavior(records []model.BehavioralEvidence) *model.BehavioralEvidence {
	for index := range records {
		if records[index].Status == behaviorObserved {
			return &records[index]
		}
	}
	return nil
}

func unresolvedBehavior(records []model.BehavioralEvidence) *model.BehavioralEvidence {
	for index := range records {
		if records[index].Status == behaviorUnresolved {
			return &records[index]
		}
	}
	return nil
}
