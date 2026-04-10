BINARY := sushiclaw
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build test install lint fmt vet deps sync-picoclaw

build:
	CGO_ENABLED=0 go build -o $(BINARY) .

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
