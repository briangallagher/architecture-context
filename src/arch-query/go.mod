module github.com/jctanner/arch-query

go 1.25.5

require (
	github.com/jctanner/arch-analyzer v0.0.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/jctanner/arch-analyzer => ../arch-analyzer

require (
	github.com/dlclark/regexp2 v1.11.5
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/text v0.24.0 // indirect
)
