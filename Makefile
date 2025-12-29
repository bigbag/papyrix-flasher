VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build build-all clean test fmt lint release

# Build for current platform
build:
	go build $(LDFLAGS) -o bin/papyrix-flasher ./cmd/papyrix-flasher

# Build for all platforms
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-linux-amd64 ./cmd/papyrix-flasher
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/papyrix-flasher-linux-arm64 ./cmd/papyrix-flasher

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-darwin-amd64 ./cmd/papyrix-flasher
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/papyrix-flasher-darwin-arm64 ./cmd/papyrix-flasher

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/papyrix-flasher-windows-amd64.exe ./cmd/papyrix-flasher

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Create release archives
release: build-all
	mkdir -p release
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-linux-amd64.tar.gz papyrix-flasher-linux-amd64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-linux-arm64.tar.gz papyrix-flasher-linux-arm64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-darwin-amd64.tar.gz papyrix-flasher-darwin-amd64
	cd bin && tar czf ../release/papyrix-flasher-$(VERSION)-darwin-arm64.tar.gz papyrix-flasher-darwin-arm64
	cd bin && zip ../release/papyrix-flasher-$(VERSION)-windows-amd64.zip papyrix-flasher-windows-amd64.exe

# Update embedded binaries from papyrix-reader
update-embedded:
	@if [ -d "../papyrix-reader/.pio/build/default" ]; then \
		cp ../papyrix-reader/.pio/build/default/bootloader.bin embedded/; \
		cp ../papyrix-reader/.pio/build/default/partitions.bin embedded/; \
		echo "Updated embedded binaries from papyrix-reader"; \
	else \
		echo "Error: papyrix-reader build not found. Run 'make build' in papyrix-reader first."; \
		exit 1; \
	fi

# Install locally
install: build
	cp bin/papyrix-flasher $(GOPATH)/bin/ || cp bin/papyrix-flasher /usr/local/bin/
