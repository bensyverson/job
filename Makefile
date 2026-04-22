BINARY := job
PKG    := ./cmd/job

.PHONY: build install run test fmt fix vet clean help

build:
	go build -o $(BINARY) $(PKG)

install:
	go install $(PKG)

# Usage: make run ARGS="list --mine"
run:
	go run $(PKG) $(ARGS)

test:
	go test ./...

fmt:
	go fmt ./...

fix:
	go fix ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

help:
	@echo "Targets:"
	@echo "  build    - compile ./$(BINARY) from $(PKG)"
	@echo "  install  - go install to \$$GOBIN"
	@echo "  run      - go run (pass args via ARGS=\"...\")"
	@echo "  test     - run all tests"
	@echo "  fmt      - go fmt ./..."
	@echo "  fix      - go fix ./..."
	@echo "  vet      - go vet ./..."
	@echo "  clean    - remove the local binary"
