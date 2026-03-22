GOFLAGS ?=
GOARCH_ARM64 = arm64
GOOS_LINUX   = linux
TARGET_ARCH  ?= $(shell go env GOARCH)

GOLANGCI_LINT_VERSION = v2.11.3
TOOLS_BIN_DIR         = bin/tools-$(shell go env GOVERSION)
GOLANGCI_LINT         = $(TOOLS_BIN_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)

SERVICES = core ui dslr buzzer gallery ctl

PROTO_DIRS = proto/core proto/ui proto/dslr

.PHONY: all build build-arm64 generate test vet lint golangci-lint image run-dummy clean $(addprefix image-,$(SERVICES))

all: build

## Generate Go code from proto files
generate:
	@echo "==> Generating protobuf/gRPC stubs"
	@for dir in $(PROTO_DIRS); do \
		protoc \
			--go_out=. \
			--go_opt=paths=source_relative \
			--go-grpc_out=. \
			--go-grpc_opt=paths=source_relative \
			$$dir/*.proto; \
	done

## Build all service binaries for the host architecture
build:
	@echo "==> Building binaries (host arch)"
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "  cmd/$$svc"; \
		CGO_ENABLED=0 go build $(GOFLAGS) -o bin/$$svc ./cmd/$$svc; \
	done
	@# dslr and ui require CGO
	@echo "  cmd/dslr (CGO)"
	CGO_ENABLED=1 go build $(GOFLAGS) -o bin/dslr ./cmd/dslr
	@echo "  cmd/ui (CGO)"
	CGO_ENABLED=1 go build $(GOFLAGS) -o bin/ui ./cmd/ui

## Build all service binaries for arm64 (Raspberry Pi 5)
build-arm64:
	@echo "==> Building binaries for arm64"
	@mkdir -p bin/arm64
	@for svc in core buzzer gallery ctl; do \
		echo "  cmd/$$svc"; \
		CGO_ENABLED=0 GOARCH=$(GOARCH_ARM64) GOOS=$(GOOS_LINUX) \
			go build $(GOFLAGS) -o bin/arm64/$$svc ./cmd/$$svc; \
	done
	@# dslr and ui require CGO — cross-compile with aarch64 toolchain
	@echo "  cmd/dslr (CGO arm64)"
	CGO_ENABLED=1 GOARCH=$(GOARCH_ARM64) GOOS=$(GOOS_LINUX) \
		CC=aarch64-linux-gnu-gcc \
		go build $(GOFLAGS) -o bin/arm64/dslr ./cmd/dslr
	@echo "  cmd/ui (CGO arm64)"
	CGO_ENABLED=1 GOARCH=$(GOARCH_ARM64) GOOS=$(GOOS_LINUX) \
		CC=aarch64-linux-gnu-gcc \
		go build $(GOFLAGS) -o bin/arm64/ui ./cmd/ui

## Run tests
test:
	go test ./...

## Run go vet
vet:
	go vet ./...

## Install golangci-lint to bin/tools-<goversion>/golangci-lint-<version>
$(GOLANGCI_LINT):
	@mkdir -p $(TOOLS_BIN_DIR)
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install \
		github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv $(TOOLS_BIN_DIR)/golangci-lint $(GOLANGCI_LINT)

golangci-lint: $(GOLANGCI_LINT)

## Run golangci-lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

## Build all container images
image: $(addprefix image-,$(SERVICES))

## Build a single container image: make image-<service>
$(addprefix image-,$(SERVICES)): image-%:
	@echo "==> Building gophotobooth-$* ($(TARGET_ARCH))"
	podman build --platform linux/$(TARGET_ARCH) -f Dockerfile.$* -t gophotobooth-$* .

## Start all services locally with dummy camera + ctl trigger for debugging
run-dummy:
	podman compose --file compose.yaml --profile no-hardware up

clean:
	rm -rf bin/
	@for dir in $(PROTO_DIRS); do \
		rm -f $$dir/*.pb.go; \
	done
