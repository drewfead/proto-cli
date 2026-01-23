# Makefile for proto-cli development

# Binary output directory (gitignored)
BIN_DIR := bin

# Installation directory (can be overridden via command line)
INSTALL_LOCATION ?= ~/bin

##@ Build

.PHONY: build
build: build/gen build/example ## Build all binaries

.PHONY: build/gen
build/gen: ## Build the protoc-gen-cli code generator
	@echo "Building protoc-gen-cli..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/protoc-gen-cli ./cmd/gen
	@echo "✓ Built: $(BIN_DIR)/protoc-gen-cli"

.PHONY: build/example
build/example: generate ## Build the example usercli binary (generates proto files first)
	@echo "Building example usercli..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/usercli ./examples/simple/usercli
	@echo "✓ Built: $(BIN_DIR)/usercli"

.PHONY: install
install: build/gen ## Build and install protoc-gen-cli to ~/bin (override with INSTALL_LOCATION=/path)
	@echo "Installing protoc-gen-cli to $(INSTALL_LOCATION)..."
	@mkdir -p $(INSTALL_LOCATION)
	cp $(BIN_DIR)/protoc-gen-cli $(INSTALL_LOCATION)/protoc-gen-cli
	@echo "✓ Installed: $(INSTALL_LOCATION)/protoc-gen-cli"

.PHONY: clean
clean: ## Clean build artifacts and generated proto files
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR)/
	rm -f internal/clipb/*.pb.go
	rm -f examples/simple/*.pb.go
	go clean
	@echo "✓ Clean complete"

##@ Proto

.PHONY: generate
generate: ## Generate proto files using buf
	@echo "Generating proto files..."
	go run github.com/bufbuild/buf/cmd/buf generate
	@echo "✓ Proto generation complete"

.PHONY: generate/clean
generate/clean: ## Clean and regenerate all proto files
	@echo "Cleaning generated proto files..."
	rm -f internal/clipb/*.pb.go
	rm -f examples/simple/*.pb.go
	@echo "Regenerating proto files..."
	go run github.com/bufbuild/buf/cmd/buf generate
	@echo "✓ Clean regeneration complete"

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

.PHONY: test/example
test/example: build/example ## Build and test the example CLI
	@echo "Testing example CLI..."
	@echo "  Testing direct call..."
	@$(BIN_DIR)/usercli userservice getuser --id 1 --format json > /dev/null 2>&1 && echo "  ✓ Direct call works" || (echo "  ✗ Direct call failed" && exit 1)
	@echo "  Testing daemon..."
	@$(BIN_DIR)/usercli daemonize --port 50099 > /tmp/test-daemon.log 2>&1 & \
		DAEMON_PID=$$! && \
		sleep 2 && \
		$(BIN_DIR)/usercli userservice getuser --id 1 --format json --remote localhost:50099 > /dev/null 2>&1 && \
		kill $$DAEMON_PID 2>/dev/null && \
		echo "  ✓ Remote call works" || \
		(kill $$DAEMON_PID 2>/dev/null; echo "  ✗ Remote call failed" && exit 1)
	@echo "✓ Example tests passed"

##@ Lint

.PHONY: lint
lint: ## Run linter on all files
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run ./...

.PHONY: fmt
fmt: ## Auto-format code
	go run github.com/golangci/golangci-lint/cmd/golangci-lint fmt ./...
	go fmt ./...

##@ Development

.PHONY: dev/setup
dev/setup: ## Set up development environment
	@echo "Setting up development environment..."
	go mod download
	go mod tidy
	@echo "✓ Development environment ready"

.PHONY: dev/example
dev/example: generate build/example ## Generate proto and build example for quick testing
	@echo "✓ Example ready for testing"
	@echo ""
	@echo "Try these commands:"
	@echo "  $(BIN_DIR)/usercli userservice getuser --id 1 --format json"
	@echo "  $(BIN_DIR)/usercli daemonize --port 50051"

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
