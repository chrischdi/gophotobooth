#!/usr/bin/env bash
# flash-raspberrypi-os.sh — Download, flash, and configure Raspberry Pi OS for gophotobooth
#
# Usage: bash scripts/flash-raspberrypi-os.sh <device> [options]
#
# Arguments:
#   <device>                      Block device to flash (e.g. /dev/sda, /dev/nvme0n1)
#   --package <tarball>           Path to gophotobooth-arm64.tar.gz (from make package).
#                                 Copied to the root partition and extracted on first boot
#                                 before install.sh runs.
#   --hostname <name>             System hostname (default: fotobox)
#   --user <username> <password>  OS user account (default: pi / raspberry).
#   --wifi <ssid> <password>      Pre-configure a WiFi network (repeatable).
#                                 Written as a NetworkManager connection file.
#                                 WARNING: password is stored as plain text in the image.
#   --keep-image                  Keep the downloaded .img.xz in /tmp after flashing.
#
# The script will:
#   1. Download the latest Raspberry Pi OS Lite (arm64) image
#   2. Flash it to <device>
#   3. Resize partition 2 (ext4 root) to 32 GiB
#   4. Add partition 3 (exFAT, remaining space) for photo storage
#   5. Inject firstrun.sh + firstrun.service and WiFi config on the root partition
#   6. Write userconf.txt, ssh, config.txt tweaks and install.sh to the boot partition

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

IMAGE_URL="https://downloads.raspberrypi.com/raspios_lite_arm64_latest"
ROOT_PART_SIZE="32GiB"

# ---- parse arguments ----------------------------------------------------------------

DEVICE=""
PACKAGE=""
KEEP_IMAGE=false
HOSTNAME_VAL="fotobox"
USER_NAME="pi"
USER_PASSWORD="raspberry"
WIFI_SSIDS=()
WIFI_PASSWORDS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --package)
            [[ $# -ge 2 ]] || { echo "ERROR: --package requires an argument" >&2; exit 1; }
            PACKAGE="$2"
            shift 2
            ;;
        --hostname)
            [[ $# -ge 2 ]] || { echo "ERROR: --hostname requires an argument" >&2; exit 1; }
            HOSTNAME_VAL="$2"
            shift 2
            ;;
        --user)
            [[ $# -ge 3 ]] || { echo "ERROR: --user requires <username> and <password>" >&2; exit 1; }
            USER_NAME="$2"
            USER_PASSWORD="$3"
            shift 3
            ;;
        --wifi)
            [[ $# -ge 3 ]] || { echo "ERROR: --wifi requires <ssid> and <password>" >&2; exit 1; }
            WIFI_SSIDS+=("$2")
            WIFI_PASSWORDS+=("$3")
            shift 3
            ;;
        --keep-image)
            KEEP_IMAGE=true
            shift
            ;;
        -*)
            echo "ERROR: Unknown option: $1" >&2
            exit 1
            ;;
        *)
            if [[ -z "$DEVICE" ]]; then
                DEVICE="$1"
            else
                echo "ERROR: Unexpected argument: $1" >&2
                exit 1
            fi
            shift
            ;;
    esac
done

# ---- helpers ------------------------------------------------------------------------

info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

# Returns the partition device path for a given disk and partition number.
# /dev/sda   → /dev/sda1  (no trailing digit in disk name)
# /dev/nvme0n1 / /dev/mmcblk0 → /dev/nvme0n1p1 / /dev/mmcblk0p1  (trailing digit → append 'p')
part() {
    local dev="$1" num="$2"
    if [[ "$dev" =~ [0-9]$ ]]; then
        echo "${dev}p${num}"
    else
        echo "${dev}${num}"
    fi
}

MOUNT_ROOT=""
MOUNT_BOOT=""
IMAGE_PATH=""

cleanup() {
    local rc=$?
    [[ -n "$MOUNT_BOOT" ]] && sudo umount "$MOUNT_BOOT" 2>/dev/null || true
    [[ -n "$MOUNT_ROOT" ]] && sudo umount "$MOUNT_ROOT" 2>/dev/null || true
    [[ -n "$MOUNT_BOOT" ]] && rmdir "$MOUNT_BOOT" 2>/dev/null || true
    [[ -n "$MOUNT_ROOT" ]] && rmdir "$MOUNT_ROOT" 2>/dev/null || true
    if [[ "$KEEP_IMAGE" == false && -n "$IMAGE_PATH" && -f "$IMAGE_PATH" ]]; then
        rm -f "$IMAGE_PATH"
    fi
    exit $rc
}
trap cleanup EXIT INT TERM

# ---- 1. validate inputs -------------------------------------------------------------

[[ -n "$DEVICE" ]] || error "Usage: $0 <device> [--package <tarball>] [--keep-image]"
[[ -b "$DEVICE" ]] || error "$DEVICE is not a block device"
[[ -n "$PACKAGE" && ! -f "$PACKAGE" ]] && error "Package not found: $PACKAGE"

for tool in curl xzcat dd mount partprobe parted e2fsck resize2fs mkfs.exfat openssl; do
    command -v "$tool" &>/dev/null || error "Required tool not found: $tool (install the package that provides it)"
done

echo ""
echo "  Device:   $DEVICE"
echo "  Hostname: $HOSTNAME_VAL"
echo "  User:     $USER_NAME"
echo "  Package:  ${PACKAGE:-none}"
echo ""
echo "WARNING: ALL DATA ON $DEVICE WILL BE DESTROYED."
echo -n "Type 'yes' to continue: "
read -r CONFIRM
[[ "$CONFIRM" == "yes" ]] || { echo "Aborted."; exit 0; }
echo ""

# ---- 2. download image --------------------------------------------------------------

info "Resolving latest Raspberry Pi OS Lite (arm64) URL..."
RESOLVED_URL=$(curl -sI -L --max-redirs 10 -o /dev/null -w '%{url_effective}' "$IMAGE_URL")
IMAGE_FILENAME=$(basename "$RESOLVED_URL")
IMAGE_PATH="/tmp/$IMAGE_FILENAME"

if [[ -f "$IMAGE_PATH" ]]; then
    info "Using cached image: $IMAGE_PATH"
else
    info "Downloading $IMAGE_FILENAME..."
    curl -L --progress-bar -o "$IMAGE_PATH" "$IMAGE_URL"
fi

# ---- 3. flash to device -------------------------------------------------------------

info "Flashing $IMAGE_FILENAME to $DEVICE (this will take a while)..."
xzcat "$IMAGE_PATH" | sudo dd of="$DEVICE" bs=4M status=progress conv=fsync
sudo sync

info "Re-reading partition table..."
sudo partprobe "$DEVICE"
sleep 2

# ---- 4. repartition -----------------------------------------------------------------

info "Resizing partition 2 (ext4 root) to $ROOT_PART_SIZE..."
sudo parted -s "$DEVICE" resizepart 2 "$ROOT_PART_SIZE"
sudo partprobe "$DEVICE"
sleep 1

info "Checking and resizing ext4 filesystem on $(part "$DEVICE" 2)..."
sudo e2fsck -f "$(part "$DEVICE" 2)"
sudo resize2fs "$(part "$DEVICE" 2)"

info "Creating partition 3 (exFAT, remaining disk space)..."
sudo parted -s "$DEVICE" mkpart primary "$ROOT_PART_SIZE" 100%
sudo partprobe "$DEVICE"
sleep 1

info "Formatting partition 3 as exFAT (label: photos)..."
sudo mkfs.exfat -n photos "$(part "$DEVICE" 3)"

DATA_UUID=$(sudo blkid -s UUID -o value "$(part "$DEVICE" 3)")
[[ -n "$DATA_UUID" ]] || error "Could not read UUID of $(part "$DEVICE" 3)"

# ---- 5. mount root partition --------------------------------------------------------

MOUNT_ROOT=$(mktemp -d)
info "Mounting root partition ($(part "$DEVICE" 2)) at $MOUNT_ROOT..."
sudo mount "$(part "$DEVICE" 2)" "$MOUNT_ROOT"

info "Creating /data mount point and adding fstab entry (UUID=$DATA_UUID)..."
sudo mkdir -p "$MOUNT_ROOT/data"
echo "UUID=${DATA_UUID} /data exfat defaults,uid=0,gid=0,umask=0 0 0" | sudo tee -a "$MOUNT_ROOT/etc/fstab" > /dev/null

if [[ -n "$PACKAGE" ]]; then
    info "Copying package to root partition (/root/gophotobooth.tar.gz)..."
    sudo cp "$PACKAGE" "$MOUNT_ROOT/root/gophotobooth.tar.gz"
fi

info "Installing firstrun.service..."
sudo mkdir -p "$MOUNT_ROOT/etc/systemd/system/multi-user.target.wants"
sudo tee "$MOUNT_ROOT/etc/systemd/system/firstrun.service" > /dev/null <<'UNIT'
[Unit]
Description=First-boot gophotobooth setup
ConditionPathExists=/boot/firmware/firstrun.sh
After=network.target

[Service]
Type=oneshot
ExecStart=/boot/firmware/firstrun.sh
ExecStartPost=/bin/systemctl disable firstrun.service
StandardOutput=journal+console
StandardError=journal+console
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNIT

sudo ln -sf /etc/systemd/system/firstrun.service \
    "$MOUNT_ROOT/etc/systemd/system/multi-user.target.wants/firstrun.service"

if [[ ${#WIFI_SSIDS[@]} -gt 0 ]]; then
    info "Writing NetworkManager WiFi connection files..."
    sudo mkdir -p "$MOUNT_ROOT/etc/NetworkManager/system-connections"
    for i in "${!WIFI_SSIDS[@]}"; do
        local_ssid="${WIFI_SSIDS[$i]}"
        local_pass="${WIFI_PASSWORDS[$i]}"
        local_file="$MOUNT_ROOT/etc/NetworkManager/system-connections/${local_ssid}.nmconnection"
        sudo tee "$local_file" > /dev/null <<EOF
[connection]
id=${local_ssid}
type=wifi
autoconnect=true

[wifi]
mode=infrastructure
ssid=${local_ssid}

[wifi-security]
auth-alg=open
key-mgmt=wpa-psk
psk=${local_pass}

[ipv4]
method=auto

[ipv6]
addr-gen-mode=default
method=auto
EOF
        sudo chmod 600 "$local_file"
    done

    info "Enabling NetworkManager..."
    sudo ln -sf /lib/systemd/system/NetworkManager.service \
        "$MOUNT_ROOT/etc/systemd/system/multi-user.target.wants/NetworkManager.service"
fi

sudo umount "$MOUNT_ROOT"
rmdir "$MOUNT_ROOT"
MOUNT_ROOT=""

# ---- 6. mount boot partition --------------------------------------------------------

MOUNT_BOOT=$(mktemp -d)
info "Mounting boot partition ($(part "$DEVICE" 1)) at $MOUNT_BOOT..."
sudo mount "$(part "$DEVICE" 1)" "$MOUNT_BOOT"

info "Writing userconf.txt (pre-configures user '$USER_NAME' to skip setup wizard)..."
hashed=$(echo "$USER_PASSWORD" | openssl passwd -6 -stdin)
echo "${USER_NAME}:${hashed}" | sudo tee "$MOUNT_BOOT/userconf.txt" > /dev/null

info "Enabling SSH (creating /boot/firmware/ssh)..."
sudo touch "$MOUNT_BOOT/ssh"

info "Setting usb_max_current_enable=1 in config.txt..."
if grep -q '^usb_max_current_enable=1' "$MOUNT_BOOT/config.txt" 2>/dev/null; then
    info "usb_max_current_enable=1 already set — skipping."
else
    echo 'usb_max_current_enable=1' | sudo tee -a "$MOUNT_BOOT/config.txt" > /dev/null
fi

info "Setting framebuffer_depth=32 in config.txt..."
if grep -q '^framebuffer_depth=32' "$MOUNT_BOOT/config.txt" 2>/dev/null; then
    info "framebuffer_depth=32 already set — skipping."
else
    echo 'framebuffer_depth=32' | sudo tee -a "$MOUNT_BOOT/config.txt" > /dev/null
fi

info "Copying install.sh to boot partition..."
sudo cp "$REPO_ROOT/scripts/install.sh" "$MOUNT_BOOT/install.sh"
sudo chmod +x "$MOUNT_BOOT/install.sh"

if [[ -n "$PACKAGE" ]]; then
    info "Writing firstrun.sh (extracts package then runs install.sh)..."
    sudo tee "$MOUNT_BOOT/firstrun.sh" > /dev/null <<FIRSTRUN
#!/usr/bin/env bash
set -euo pipefail
mkdir -p /root/gophotobooth
tar -xzf /root/gophotobooth.tar.gz -C /root/gophotobooth --strip-components=1
rm -f /root/gophotobooth.tar.gz
cp /boot/firmware/install.sh /root/gophotobooth/scripts/install.sh
bash /root/gophotobooth/scripts/install.sh --hostname ${HOSTNAME_VAL}
rm -f /boot/firmware/firstrun.sh /boot/firmware/install.sh
FIRSTRUN
else
    info "Writing firstrun.sh (calls install.sh directly)..."
    sudo tee "$MOUNT_BOOT/firstrun.sh" > /dev/null <<FIRSTRUN
#!/usr/bin/env bash
set -euo pipefail
bash /boot/firmware/install.sh --hostname ${HOSTNAME_VAL}
rm -f /boot/firmware/firstrun.sh /boot/firmware/install.sh
FIRSTRUN
fi

sudo chmod +x "$MOUNT_BOOT/firstrun.sh"

sudo umount "$MOUNT_BOOT"
rmdir "$MOUNT_BOOT"
MOUNT_BOOT=""

# ---- 7. done ------------------------------------------------------------------------

echo ""
echo "=============================="
echo "Flash complete!"
echo ""
echo "  Device:      $DEVICE"
echo "  Hostname:    $HOSTNAME_VAL"
echo "  User:        $USER_NAME (SSH enabled, wizard skipped)"
echo "  Partitions:"
echo "    1  FAT32   boot (unchanged)"
echo "    2  ext4    root (resized to $ROOT_PART_SIZE)"
echo "    3  exFAT   photos (remaining space)"
echo ""
if [[ -n "$PACKAGE" ]]; then
    echo "  First boot: install.sh will run automatically via firstrun.service."
else
    echo "  First boot: provision images.tar on the Pi, then reboot or run:"
    echo "    sudo bash /boot/firmware/firstrun.sh"
fi
echo ""
echo "  Insert the SSD into your Raspberry Pi 5 and power it on."
echo "  Monitor installation: journalctl -u firstrun -f"
echo "=============================="
