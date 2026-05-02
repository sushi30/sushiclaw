BINARY      := sushiclaw
INSTALL_DIR := $(HOME)/.local/bin
VERSION     ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo dev)
BUILDTIME   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOVER       := $(shell go version | awk '{print $$3}')
VERSION_PKG := github.com/sushi30/sushiclaw/internal/version
LDFLAGS     := -X $(VERSION_PKG).Version=$(VERSION) \
               -X $(VERSION_PKG).GitCommit=$(COMMIT) \
               -X $(VERSION_PKG).BuildTime=$(BUILDTIME) \
               -X $(VERSION_PKG).GoVersion=$(GOVER)

.PHONY: build test install lint fmt vet deps test-integration release-check publish-release publish-version publish-version-dry-run air

build:
	CGO_ENABLED=0 go build -tags whatsapp_native -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

test-cover:
	@go test -coverprofile=coverage.osut ./... > /dev/null 2>&1 || true
	@echo "---"
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out

coverage-html: test-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

coverage: coverage-html

install: build
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

deps:
	go mod tidy

test-integration:
	go test -v -run 'TestEmailInboundPipeline|TestEmailOutboundPipeline' ./pkg/channels/email/...

release-check:
	@test "$(VERSION)" != "dev" || (echo "ERROR: no git tag found, set VERSION= explicitly" && exit 1)
	@echo "Releasing $(VERSION) from commit $(COMMIT)"

publish-release: publish-version

publish-version:
	./scripts/publish-version.sh

publish-version-dry-run:
	./scripts/publish-version.sh --dry-run

air:
	./script/deh.sh
