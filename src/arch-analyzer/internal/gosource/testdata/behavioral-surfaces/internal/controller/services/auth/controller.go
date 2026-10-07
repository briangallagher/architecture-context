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

func (h *ServiceHandler) NewReconciler(ctx context.Context, mgr ctrl.Manager) error {
	_, err := reconciler.ReconcilerFor(mgr, &serviceapi.Auth{}).
		Watches(
			&corev1.Namespace{},
			reconciler.WithEventHandler(handlers.ToNamed(serviceapi.AuthInstanceName)),
			reconciler.WithPredicates(resources.CreatedOrUpdatedOrDeletedNamed("models-as-a-service")),
		).
		Watches(
			&corev1.Namespace{},
			reconciler.WithEventHandler(handlers.ToNamed(serviceapi.AuthInstanceName)),
			reconciler.WithPredicates(resources.CreatedOrUpdatedOrDeletedNamed("kuadrant-system")),
		).
		Build(ctx)
	return err
}
