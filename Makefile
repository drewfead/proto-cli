# Makefile for proto-cli development

# Binary output directory (gitignored)
BIN_DIR := bin

# Installation directory (can be overridden via command line)
INSTALL_LOCATION ?= ~/bin

##@ Build

.PHONY: build
build: build/gen build/example build/streaming ## Build all binaries

.PHONY: build/gen
build/gen: ## Build the proto-cli-gen code generator
	@echo "Building proto-cli-gen..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/proto-cli-gen ./cmd/proto-cli-gen
	@echo "✓ Built: $(BIN_DIR)/proto-cli-gen"

.PHONY: build/example
build/example: generate ## Build the example usercli binary (generates proto files first)
	@echo "Building example usercli..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/usercli ./examples/simple/usercli
	@echo "✓ Built: $(BIN_DIR)/usercli"

.PHONY: build/streaming
build/streaming: generate ## Build the streaming example streamcli binary
	@echo "Building streaming example streamcli..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/streamcli ./examples/streaming/streamcli
	@echo "✓ Built: $(BIN_DIR)/streamcli"

.PHONY: install
install: build/gen ## Build and install proto-cli-gen to ~/bin (override with INSTALL_LOCATION=/path)
	@echo "Installing proto-cli-gen to $(INSTALL_LOCATION)..."
	@mkdir -p $(INSTALL_LOCATION)
	cp $(BIN_DIR)/proto-cli-gen $(INSTALL_LOCATION)/proto-cli-gen
	@echo "✓ Installed: $(INSTALL_LOCATION)/proto-cli-gen"

.PHONY: clean
clean: ## Clean build artifacts and generated proto files
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR)/
	rm -f internal/clipb/*.pb.go
	rm -f examples/simple/*.pb.go
	rm -f examples/streaming/*.pb.go
	go clean
	@echo "✓ Clean complete"

##@ Proto

.PHONY: generate
generate: ## Generate proto files using buf
	@echo "Generating proto files..."
	go tool buf generate --template buf.gen.examples.yaml
	go generate ./...
	@echo "✓ Proto generation complete"

.PHONY: generate/clean
generate/clean: ## Clean and regenerate all proto files
	@echo "Cleaning generated proto files..."
	rm -f internal/clipb/*.pb.go
	rm -f examples/simple/*.pb.go
	rm -f examples/streaming/*.pb.go
	@echo "Regenerating proto files..."
	go generate ./...
	@echo "✓ Clean regeneration complete"

.PHONY: publish
publish: lint/proto ## Publish proto module to Buf Schema Registry
	@echo "Publishing to Buf Schema Registry (buf.build/fernet/proto-cli)..."
	go tool buf push
	@echo "✓ Published to BSR"

##@ Test

.PHONY: test
test: ## Run all tests
	go test -v -race ./...

.PHONY: test/unit
test/unit: ## Run unit tests only
	go test -v -race -run "^TestUnit_" ./...

.PHONY: test/integration
test/integration: ## Run integration tests only
	go test -v -race -run "^TestIntegration_" ./...

##@ Lint

.PHONY: lint
lint: lint/go lint/proto lint/fmt ## Run all linters and format checks

.PHONY: lint/go
lint/go: ## Run Go linter
	go tool golangci-lint run ./...

.PHONY: lint/proto
lint/proto: ## Run proto linter (buf lint)
	go tool buf lint

.PHONY: fmt
fmt: ## Auto-format code
	@echo "Formatting Go code..."
	go tool golangci-lint fmt ./...
	go tool gofumpt -l -w .
	@echo "Formatting proto files..."
	go tool buf format -w
	@echo "✓ Formatting complete"

.PHONY: lint/fmt
lint/fmt: ## Check if code is properly formatted
	@echo "Checking Go formatting..."
	@if [ -n "$$(go tool gofumpt -l .)" ]; then \
		echo "The following files need formatting:"; \
		go tool gofumpt -l .; \
		echo "Run 'make fmt' to fix"; \
		exit 1; \
	fi
	@echo "Checking proto formatting..."
	@go tool buf format -d --exit-code
	@echo "✓ All files are properly formatted"

##@ Misc.

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help: ## Display usage help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9\/-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
