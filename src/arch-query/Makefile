.PHONY: build build-embedded test clean clean-embedded smoke smoke-embedded lint lint-fix

BINARY    := arch-query
BUILD_DIR := ../../bin
ARCH_DIR  := ../../architecture
GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) .

build-embedded:
	rm -rf _embedded/architecture
	mkdir -p _embedded/architecture
	go run ./internal/embeddeddata/cmd \
		-source "$(ARCH_DIR)" \
		-destination _embedded/architecture \
		-overlays "$(ARCH_DIR)/../overlays"
	mkdir -p $(BUILD_DIR)
	go build -tags embedded -o $(BUILD_DIR)/$(BINARY) .
	rm -rf _embedded

test:
	go test ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
	go clean

clean-embedded:
	rm -rf _embedded

smoke: build
	$(BUILD_DIR)/$(BINARY) --base-dir $(ARCH_DIR) versions
	@echo "---"
	$(BUILD_DIR)/$(BINARY) --base-dir $(ARCH_DIR) list --names-only | wc -l
	@echo "---"
	$(BUILD_DIR)/$(BINARY) --base-dir $(ARCH_DIR) component kserve
	@echo "---"
	$(BUILD_DIR)/$(BINARY) --base-dir $(ARCH_DIR) search vllm
	@echo "---"
	$(BUILD_DIR)/$(BINARY) --base-dir $(ARCH_DIR) --version current-ga list --names-only | wc -l

smoke-embedded: build-embedded
	@echo "=== Embedded mode (no --base-dir, running from /tmp) ==="
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) versions
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) list --names-only | wc -l
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) component kserve
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) search vllm
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) --version current-ga list --names-only | wc -l
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) webhooks | head -5
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) watches | head -5
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) provenance --upstream-only | head -5
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) schemas | head -5
	@echo "---"
	cd /tmp && $(CURDIR)/$(BUILD_DIR)/$(BINARY) images | head -5

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...
