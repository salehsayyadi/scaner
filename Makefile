# Piyazche Makefile

BINARY_NAME=piyazche
VERSION?=2.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
# -checklinkname=0 is required to build xray-core's sing dependency on Go 1.23+
LDFLAGS=-ldflags="-s -w -checklinkname=0 -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

.PHONY: all build clean deps test run help install

all: deps build

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

## build-static: Build static binary (no CGO)
build-static:
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

## build-linux: Cross-compile for Linux amd64
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .

## build-windows: Cross-compile for Windows amd64
build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

## build-darwin: Cross-compile for macOS amd64
build-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .

## build-all: Build for all platforms
build-all: build-linux build-windows build-darwin

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-linux-*
	rm -f $(BINARY_NAME)-windows-*
	rm -f $(BINARY_NAME)-darwin-*

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## test: Run tests
test:
	$(GOTEST) -v ./...

## run: Build and run with default options
run: build
	./$(BINARY_NAME) --help

## install: Install to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) .

## fmt: Format code
fmt:
	$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	$(GOCMD) vet ./...

## lint: Run linter (requires golangci-lint)
lint:
	golangci-lint run

## help: Show this help
help:
	@echo "CFScanner - Cloudflare IP Scanner"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
