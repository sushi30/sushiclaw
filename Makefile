BINARY := sushiclaw
INSTALL_DIR := $(HOME)/.local/bin

GIT_COMMIT := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
VERSION_PKG := github.com/sushi30/sushiclaw/internal/version
LDFLAGS := -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT)

.PHONY: build test install lint fmt vet deps sync-picoclaw test-integration

build:
	CGO_ENABLED=0 go build -tags whatsapp_native -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

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

sync-picoclaw:
	git submodule update --remote picoclaw
	go mod tidy

test-integration:
	go test -v -run 'TestEmailInboundPipeline|TestEmailOutboundPipeline' ./pkg/channels/email/...
