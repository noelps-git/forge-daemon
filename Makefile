BINARY     = forge
VERSION    = 0.1.0
BUILD_DIR  = bin
PKG        = github.com/forgeapp/forge-daemon/cmd/forge
LDFLAGS    = -ldflags "-X main.Version=$(VERSION)"

.PHONY: build dev test test-race fmt lint ci clean install

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(PKG)

dev:
	CGO_ENABLED=1 go run $(LDFLAGS) $(PKG) start

test:
	CGO_ENABLED=1 go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

fmt:
	gofmt -w .
	goimports -w . 2>/dev/null || true

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

ci: fmt lint test-race

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
