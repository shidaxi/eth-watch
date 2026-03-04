.PHONY: build test clean snapshot release install-tools

BINARY_NAME := eth-watch

build:
	go build -o $(BINARY_NAME) .

test:
	go test ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist

# Run goreleaser in snapshot mode (no publish, for CI/PR)
snapshot:
	goreleaser build --snapshot --clean

# Full release (run when tagging; uploads to GitHub Release)
release:
	goreleaser release --clean

# Install goreleaser locally
install-tools:
	go install github.com/goreleaser/goreleaser/v2@latest
