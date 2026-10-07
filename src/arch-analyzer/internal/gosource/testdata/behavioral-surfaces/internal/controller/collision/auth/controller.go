package auth

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	serviceapi "example.com/operator/api/services/v1alpha1"
	"example.com/operator/pkg/controller/handlers"
	"example.com/operator/pkg/controller/predicates/resources"
	"example.com/operator/pkg/controller/reconciler"
)

type ServiceHandler struct{}

func configuredNamespace() string { return "dynamic-namespace" }

func (h *ServiceHandler) NewReconciler(ctx context.Context, mgr ctrl.Manager) error {
	name := configuredNamespace()
	_, err := reconciler.ReconcilerFor(mgr, &serviceapi.Auth{}).
		Watches(
			&corev1.Namespace{},
			reconciler.WithEventHandler(handlers.ToNamed(serviceapi.AuthInstanceName)),
			reconciler.WithPredicates(resources.CreatedOrUpdatedOrDeletedNamed(name)),
		).
		Build(ctx)
	return err
}
