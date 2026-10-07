# Fixture Provenance

Sanitized behavioral shapes from `rhods-operator` commit
`4ada791819c522a4cda54f9029ab3e4056ed31ed`:

- `cmd/main.go` preserves the `ctrl.NewManager` options binding, the
  controller-runtime metrics options IIFE, and the `MetricsSecure` branch from
  `cmd/main.go:485-500`.
- `internal/controller/services/auth/controller.go` preserves the two literal
  Namespace watch predicates from
  `internal/controller/services/auth/auth_controller.go:63-76`.

The `dynamic` and `collision` packages are synthetic negative cases. They
exercise dynamic values, unsupported same-named wrappers, and distinct package
identities without copying generated architecture or pipeline logs.
