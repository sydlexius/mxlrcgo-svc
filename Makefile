.PHONY: build run test test-cover lint fmt hooks clean help

# Binary name
BINARY=mxlrc-go

## build: Build the binary
build:
	go build -o $(BINARY) .

## run: Build and run
run: build
	./$(BINARY)

## test: Run all tests
test:
	go test -v -race -count=1 ./...

## test-cover: Run tests with coverage
test-cover:
	go test -count=1 -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format all Go files
fmt:
	gofmt -w .
	goimports -w .

## hooks: Install git pre-commit hook (mirrors CI lint checks)
hooks:
	cp .githooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)
	rm -f coverage.out coverage.html

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
