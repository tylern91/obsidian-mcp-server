.PHONY: build test vet fmt lint run clean help

BINARY := obsidian-mcp

## build: compile the binary
build:
	go build -o $(BINARY) ./cmd/obsidian-mcp/

## test: run all tests with race detector
test:
	go test -race ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go source files (gofmt + goimports)
fmt:
	gofmt -l -w .
	goimports -l -w .

## lint: run vet and check formatting
lint: vet
	@gofmt -l . | grep . && exit 1 || true

## run: run the server (pass args via ARGS=)
run:
	go run ./cmd/obsidian-mcp/ $(ARGS)

## clean: remove compiled binary
clean:
	rm -f $(BINARY)

## help: list all available targets
help:
	@grep -E '^## [a-z]' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-10s %s\n", substr($$1,5), $$2}'
