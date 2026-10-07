package auth

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	serviceapi "example.com/operator/api/services/v1alpha1"
	"example.com/operator/pkg/controller/reconciler"
	misleading "example.com/operator/pkg/misleading"
)

type ServiceHandler struct{}

func (h *ServiceHandler) NewReconciler(ctx context.Context, mgr ctrl.Manager) error {
	_, err := reconciler.ReconcilerFor(mgr, &serviceapi.Auth{}).
		Watches(
			&corev1.Namespace{},
			reconciler.WithPredicates(misleading.CreatedOrUpdatedOrDeletedNamed("looks-literal")),
		).
		Build(ctx)
	return err
}
