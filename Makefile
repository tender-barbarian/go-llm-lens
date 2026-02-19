BINARY   := go-llm-lens
CMD      := ./cmd/server

.PHONY: all build test lint vet check clean

all: check build

build:
	go build -o $(BINARY) $(CMD)

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

vet:
	go vet ./...

check: vet lint test

clean:
	rm -f $(BINARY)
