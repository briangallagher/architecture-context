package v1alpha1

import schema "k8s.io/apimachinery/pkg/runtime/schema"

const AuthInstanceName = "auth"

var GroupVersion = schema.GroupVersion{
	Group:   "services.platform.opendatahub.io",
	Version: "v1alpha1",
}

type Auth struct{}
