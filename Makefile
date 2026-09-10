VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

READER_DIR ?= ../papyrix-reader

.PHONY: build build-all clean test fmt lint release tag update-embedded help

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

update-embedded: ## Update embedded binaries from papyrix-reader release builds
	@set -e; \
	c3="$(READER_DIR)/.pio/build/release_xteink_c3/bootloader.bin"; \
	s3="$(READER_DIR)/.pio/build/release_x4pro/bootloader.bin"; \
	c3parts="$(READER_DIR)/.pio/build/release_xteink_c3/partitions.bin"; \
	s3parts="$(READER_DIR)/.pio/build/release_x4pro/partitions.bin"; \
	preflight() { \
		file="$$1"; expected="$$2"; \
		test -s "$$file" || { echo "Error: missing or empty $$file"; exit 1; }; \
		magic=$$(od -An -tx1 -N1 "$$file" | tr -d '[:space:]'); \
		chip=$$(od -An -tu2 -j12 -N2 "$$file" | tr -d '[:space:]'); \
		test "$$magic" = "e9" || { echo "Error: invalid image magic in $$file"; exit 1; }; \
		test "$$chip" = "$$expected" || { echo "Error: $$file targets chip $$chip, expected $$expected"; exit 1; }; \
	}; \
	preflight "$$c3" 5; \
	preflight "$$s3" 9; \
	for file in "$$c3parts" "$$s3parts"; do \
		test -s "$$file" || { echo "Error: missing or empty $$file"; exit 1; }; \
		magic=$$(od -An -tx1 -N2 "$$file" | tr -d '[:space:]'); \
		test "$$magic" = "aa50" || { echo "Error: invalid partition table magic in $$file"; exit 1; }; \
	done; \
	cmp -s "$$c3parts" "$$s3parts" || { echo "Error: release partition tables differ"; exit 1; }; \
	cp "$$c3" embedded/bootloader.bin; \
	cp "$$s3" embedded/bootloader-s3.bin; \
	cp "$$c3parts" embedded/partitions.bin; \
	echo "Updated embedded binaries from $(READER_DIR)"

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
