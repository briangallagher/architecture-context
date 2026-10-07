package main

import (
	ctrl "sigs.k8s.io/controller-runtime"
	filters "sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func unresolvedMetrics(secure bool) (any, error) {
	return ctrl.NewManager(nil, ctrl.Options{Metrics: func() ctrlmetrics.Options {
		opts := ctrlmetrics.Options{SecureServing: secure}
		if secure {
			opts.FilterProvider = filters.CustomAuthenticationWrapper
		}
		return opts
	}()})
}
