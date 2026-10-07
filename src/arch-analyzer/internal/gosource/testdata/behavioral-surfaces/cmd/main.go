package main

import (
	ctrl "sigs.k8s.io/controller-runtime"
	filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type operatorConfig struct {
	MetricsAddr   string
	MetricsSecure bool
}

func newManager(oconfig operatorConfig) (any, error) {
	return ctrl.NewManager(nil, ctrl.Options{
		Metrics: func() ctrlmetrics.Options {
			opts := ctrlmetrics.Options{
				BindAddress:   oconfig.MetricsAddr,
				SecureServing: oconfig.MetricsSecure,
			}
			if oconfig.MetricsSecure {
				opts.FilterProvider = filters.WithAuthenticationAndAuthorization
			}
			return opts
		}(),
	})
}
