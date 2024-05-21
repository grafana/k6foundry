work_dir = $(shell pwd)
golangci_version = $(shell head -n 1 .golangci.yml | tr -d '\# ')
arch = $(shell go env GOARCH)
image ?= ghcr.io/grafana/xk6-disruptor:latest
agent_image ?= ghcr.io/grafana/xk6-disruptor-agent:latest

all: build

.PHONY: build
build:
	go build -o build/k6build ./cmd/k6build

# Running with -buildvcs=false works around the issue of `go list all` failing when git, which runs as root inside
# the container, refuses to operate on the disruptor source tree as it is not owned by the same user (root).
.PHONY: lint
lint:
	docker run --rm -v $(work_dir):/k6build -w /k6build -e GOFLAGS=-buildvcs=false golangci/golangci-lint:$(golangci_version) golangci-lint run

.PHONY: test
test:
	go test -race  ./...

