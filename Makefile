VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build build-all clean test fmt lint release tag help

.DEFAULT_GOAL := help

## Build:

build: ## Build for current platform
	go build $(LDFLAGS) -o bin/papyrix-flasher ./cmd/papyrix-flasher

build-all: build-linux build-darwin build-windows ## Build for all platforms

build-linux: ## Build for Linux (amd64, arm64)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-linux-amd64 ./cmd/papyrix-flasher
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/papyrix-flasher-linux-arm64 ./cmd/papyrix-flasher

build-darwin: ## Build for macOS (amd64, arm64)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-darwin-amd64 ./cmd/papyrix-flasher
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/papyrix-flasher-darwin-arm64 ./cmd/papyrix-flasher

build-windows: ## Build for Windows (amd64)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-windows-amd64.exe ./cmd/papyrix-flasher

## Development:

test: ## Run tests
	go test -v ./...

fmt: ## Format code
	go fmt ./...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

## Release:

release: build-all ## Create release archives
	mkdir -p release
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-linux-amd64.tar.gz papyrix-flasher-linux-amd64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-linux-arm64.tar.gz papyrix-flasher-linux-arm64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-darwin-amd64.tar.gz papyrix-flasher-darwin-amd64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-darwin-arm64.tar.gz papyrix-flasher-darwin-arm64
	cd bin && zip ../release/papyrix-flasher-$(VERSION)-windows-amd64.zip papyrix-flasher-windows-amd64.exe

tag: ## Create and push a version tag (triggers GitHub release)
	@read -p "Enter tag version (e.g., 1.0.0): " TAG; \
	if [[ $$TAG =~ ^[0-9]+\.[0-9]+\.[0-9]+$$ ]]; then \
		git tag -a v$$TAG -m "v$$TAG"; \
		git push origin v$$TAG; \
		echo "Tag v$$TAG created and pushed successfully."; \
	else \
		echo "Invalid tag format. Please use X.Y.Z (e.g., 1.0.0)"; \
		exit 1; \
	fi

## Maintenance:

clean: ## Clean build artifacts
	rm -rf bin/ release/

update-embedded: ## Update embedded binaries from papyrix-reader
	@if [ -d "../papyrix-reader/.pio/build/default" ]; then \
		cp ../papyrix-reader/.pio/build/default/bootloader.bin embedded/; \
		cp ../papyrix-reader/.pio/build/default/partitions.bin embedded/; \
		echo "Updated embedded binaries from papyrix-reader"; \
	else \
		echo "Error: papyrix-reader build not found. Run 'make build' in papyrix-reader first."; \
		exit 1; \
	fi

install: build ## Install locally to GOPATH or /usr/local/bin
	cp bin/papyrix-flasher $(GOPATH)/bin/ 2>/dev/null || cp bin/papyrix-flasher /usr/local/bin/

## Help:

help: ## Show this help
	@echo "Papyrix Flasher - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; section=""} \
		/^##/ { section=substr($$0, 4); next } \
		/^[a-zA-Z_-]+:.*##/ { \
			if (section != "") { printf "\n\033[1m%s\033[0m\n", section; section="" } \
			printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 \
		}' $(MAKEFILE_LIST)
	@echo ""
