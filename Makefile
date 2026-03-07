GO ?= go
GOFILES := $(shell $(GO) list -f '{{$$dir := .Dir}}{{range .GoFiles}}{{printf "%s/%s " $$dir .}}{{end}}{{range .TestGoFiles}}{{printf "%s/%s " $$dir .}}{{end}}{{range .XTestGoFiles}}{{printf "%s/%s " $$dir .}}{{end}}' ./...)
BIN_DIR ?= $(CURDIR)/.bin
GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.3.0

.PHONY: help fmt fmt-check lint lint-install vet test test-race cover tidy check release-check

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make fmt           Format Go packages in this module' \
		'  make fmt-check     Verify formatting without changing files' \
		'  make lint          Run golangci-lint' \
		'  make lint-install  Install golangci-lint into ./.bin' \
		'  make vet           Run go vet' \
		'  make test          Run unit tests' \
		'  make test-race     Run tests with the race detector' \
		'  make cover         Generate coverage.out' \
		'  make tidy          Run go mod tidy' \
		'  make check         Run the local CI-equivalent checks' \
		'  make release-check Validate release prerequisites'

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(gofmt -l $(GOFILES))"

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

lint-install:
	@mkdir -p "$(BIN_DIR)"
	GOBIN="$(BIN_DIR)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOLANGCI_LINT):
	@$(MAKE) lint-install

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...

tidy:
	$(GO) mod tidy

check: fmt-check lint vet test

release-check: check test-race
	@case "$${TAG:-}" in \
		v*) ;; \
		"") echo "TAG is optional; set TAG=vX.Y.Z to validate a specific release tag format." ;; \
		*) echo "TAG must start with v, for example TAG=v0.1.0"; exit 1 ;; \
	esac
