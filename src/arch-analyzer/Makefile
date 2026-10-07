.PHONY: build test clean smoke lint

BINARY := arch-analyzer
BUILD_DIR := ../../bin
GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) .

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	$(GOLANGCI_LINT) run ./...

smoke: build
	$(BUILD_DIR)/$(BINARY) render \
		--input ../../architecture/rhoai.next/kueue.json \
		--output /tmp/kueue-arch-analyzer.md \
		--distribution RHOAI

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
	go clean
