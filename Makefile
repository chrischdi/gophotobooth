GOFLAGS ?=
GOARCH_ARM64 = arm64
GOOS_LINUX   = linux
TARGET_ARCH  ?= $(shell go env GOARCH)
PACKAGE_ARCH ?= arm64
DIST_DIR      = dist

CONTAINER_TOOL ?= docker

GOLANGCI_LINT_VERSION = v2.11.3
TOOLS_BIN_DIR         = bin/tools-$(shell go env GOVERSION)
GOLANGCI_LINT         = $(TOOLS_BIN_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)

SERVICES = core ui dslr buzzer gallery ctl

THIRD_PARTY_IMAGES = docker.io/cloudflare/cloudflared:2026.1.2

PROTO_DIRS = proto/core proto/ui proto/dslr

.PHONY: all build build-arm64 generate test vet lint golangci-lint image package run-dummy e2e test-hardware test-hardware-buzzer test-hardware-camera test-manual clean $(addprefix image-,$(SERVICES))

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

## Run tests (CGO-free packages only; cmd/dslr and cmd/ui require CGO and are skipped)
test:
	CGO_ENABLED=0 go test $(shell CGO_ENABLED=0 go list ./... 2>/dev/null | grep -v 'cmd/dslr\b' | grep -v 'cmd/ui\b')

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
	$(CONTAINER_TOOL) build --platform linux/$(TARGET_ARCH) -f Dockerfile.$* -t gophotobooth-$* .

## Build all images for PACKAGE_ARCH (default: arm64) and bundle into a tarball
## Output: dist/gophotobooth-<arch>.tar.gz
## Contents: images.tar (podman load), compose.yaml, scripts/
package:
	@if [ "$$(uname -m)" = "x86_64" ] && [ "$(PACKAGE_ARCH)" = "arm64" ]; then \
		if [ ! -f /proc/sys/fs/binfmt_misc/qemu-aarch64 ]; then \
			echo "ERROR: binfmt_misc support for aarch64 is not available."; \
			echo "       Run: docker run --privileged --rm docker.io/tonistiigi/binfmt --install arm64"; \
			exit 1; \
		fi \
	fi
	@$(MAKE) image TARGET_ARCH=$(PACKAGE_ARCH)
	@$(MAKE) build-arm64
	@echo "==> Packaging for $(PACKAGE_ARCH)"
	@mkdir -p $(DIST_DIR)
	@stage=$$(mktemp -d); \
	$(CONTAINER_TOOL) pull --platform linux/$(PACKAGE_ARCH) $(THIRD_PARTY_IMAGES); \
	$(CONTAINER_TOOL) save -o $$stage/images.tar $(foreach svc,$(SERVICES),gophotobooth-$(svc)) $(THIRD_PARTY_IMAGES); \
	cp compose.yaml $$stage/; \
	cp env.sample $$stage/; \
	cp -r scripts/ $$stage/scripts/; \
	cp bin/$(PACKAGE_ARCH)/ctl $$stage/ctl; \
	tar -czf $(DIST_DIR)/gophotobooth-$(PACKAGE_ARCH).tar.gz -C $$stage .; \
	rm -rf $$stage
	@echo "==> $(DIST_DIR)/gophotobooth-$(PACKAGE_ARCH).tar.gz"

## Start all services locally with dummy camera + ctl trigger for debugging
run-dummy:
	$(CONTAINER_TOOL) compose --file compose.yaml --profile no-hardware up

## Run end-to-end tests using real CGO-free binaries (requires make build first)
e2e: build
	@echo "==> Running E2E tests"
	bash scripts/e2e.sh

## Run hardware-tagged Go tests (requires GPIO button + camera hardware)
test-hardware: test-hardware-buzzer test-hardware-camera

## Run hardware-tagged Go tests for the buzzer (requires GPIO button)
test-hardware-buzzer:
	@echo "==> Running hardware Go tests (buzzer)"
	CGO_ENABLED=1 go test -tags hardware -v ./cmd/buzzer/

## Run hardware-tagged Go tests for the camera (requires DSLR camera)
test-hardware-camera:
	@echo "==> Running hardware Go tests (camera)"
	CGO_ENABLED=1 go test -tags hardware -v ./cmd/dslr/

## Interactive manual test checklist for the full hardware stack
test-manual:
	@echo "==> Running manual hardware tests"
	bash scripts/test-manual.sh

clean:
	rm -rf bin/ $(DIST_DIR)/
	@for dir in $(PROTO_DIRS); do \
		rm -f $$dir/*.pb.go; \
	done
