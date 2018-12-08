DOCKER_IMAGE_NAME ?= gophotobooth
DOCKER_IMAGE_TAG  ?= $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))-$(shell date +%Y-%m-%d)-$(shell git rev-parse --short HEAD)

ARCHLINUX_ARM_ARCH ?= arm
ARCHLINUX_ARM_URL ?= http://os.archlinuxarm.org/os/ArchLinuxARM-rpi-2-latest.tar.gz
ARCHLINUX_ARM_TAR ?= ArchLinuxARM-rpi-2-latest.tar.gz
ARCHLINUX_ARM_QEMU ?= qemu-arm-static
ARCHLINUX_ARM_CC_URL ?= https://archlinuxarm.org/builder/xtools/x-tools7h.tar.xz

build:
	@echo ">> building binary"
	go build ./cmd/gophotobooth

test:
	@echo ">> running all tests"
	@go test $(shell go list ./... | grep -v /vendor/ | grep -v /poc/);

# vet vets the code.
.PHONY: vet
vet:
	@echo ">> vetting code"
	@go vet $(shell go list ./... | grep -v /vendor/ | grep -v /poc/);

build:  rpi-image-arm cross-arm-deps build-arm rpi-image-arm

build-arm: 
	mkdir -p bin
	env CC="$(CURDIR)/tmp/x-tools7h/arm-unknown-linux-gnueabihf/bin/arm-unknown-linux-gnueabihf-gcc" \
		CGO_CFLAGS="--sysroot=$(CURDIR)/tmp/rpi-arm" \
		CGO_LDFLAGS="--sysroot=$(CURDIR)/tmp/rpi-arm -v" \
		PKG_CONFIG_DIR="" \
		PKG_CONFIG_LIBDIR=$(CURDIR)/tmp/rpi-arm/usr/lib/pkgconfig:$(CURDIR)/tmp/rpi-arm/usr/share/pkgconfig \
		PKG_CONFIG_SYSROOT_DIR=$(CURDIR)/tmp/rpi-arm \
		GOARCH=arm \
		GOARM=7 \
		GOOS=linux \
		CGO_ENABLED=1 \
		go build -o bin/gophotobooth -v -x cmd/gophotobooth/main.go

cross-arm-deps: deps-arm
	echo ">> creating directory tmp"
	mkdir -p tmp
	@echo ">> downloading crosscompiler toolchain"
	wget -c $(ARCHLINUX_ARM_CC_URL) -O tmp/x-tools7h.tar.xz
	@echo ">> unpacking crosscompiler toolchain"
	mkdir -p tmp/rpi-arm
	tar -xf tmp/rpi-$(ARCHLINUX_ARM_ARCH).tar.gz -C tmp/rpi-arm
	tar -xf tmp/x-tools7h.tar.xz -C tmp/

rpi-image-arm: deps-arm
	mkdir -p bin
	@echo ">> building rpi image for $(ARCHLINUX_ARM_ARCH)"
	docker build \
	--build-arg rootfs_tar=$(ARCHLINUX_ARM_TAR) \
	--build-arg qemu_binary=$(ARCHLINUX_ARM_QEMU) \
	-t gophotobooth:$(ARCHLINUX_ARM_ARCH) .
	docker create --name gophotobooth-$(ARCHLINUX_ARM_ARCH) --entrypoint /bin/sh gophotobooth:$(ARCHLINUX_ARM_ARCH)
	docker export gophotobooth-$(ARCHLINUX_ARM_ARCH) > tmp/rpi-$(ARCHLINUX_ARM_ARCH).tar.gz
	docker rm gophotobooth-$(ARCHLINUX_ARM_ARCH)

deps-arm:
	echo ">> creating directory tmp"
	mkdir -p tmp
	echo ">> downloading archlinux armv7 image"
	wget -c $(ARCHLINUX_ARM_URL) -O tmp/$(ARCHLINUX_ARM_TAR)
	echo ">> copying $(ARCHLINUX_ARM_QEMU) static binary"
	cp $(shell which $(ARCHLINUX_ARM_QEMU)) ./tmp/

clean:
	rm -rf ./tmp ./bin
