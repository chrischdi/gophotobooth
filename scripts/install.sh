#!/usr/bin/env bash
# install.sh — install gophotobooth on Raspberry Pi OS Lite (arm64)
#
# Usage (run as root from the extracted tarball directory):
#   sudo bash install.sh [--hostname <name>]
#
# Options:
#   --hostname <name>   Set the system hostname (default: fotobox)
#
# What it does:
#   1. Sets hostname
#   2. Enables NetworkManager and unblocks WiFi
#   3. Installs Xorg, podman, and network-manager
#   4. Installs Tailscale (via official install script)
#   5. Creates /etc/gophotobooth/env for per-site configuration
#   6. Loads container images from images.tar
#   7. Installs and enables systemd units for all services
#
# After installation, edit /etc/gophotobooth/env to set:
#   GALLERY_BASE_URL  — public URL of this Pi, e.g. http://192.168.1.42:8080
#   BUZZER_PIN        — GPIO pin name, e.g. GPIO17  (leave empty to disable buzzer)
#   PHOTOS_DIR        — path for photo storage   (default: /var/lib/gophotobooth/photos)
#   CACHE_DIR         — path for thumbnail cache (default: /var/lib/gophotobooth/cache)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARBALL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Arguments
# ---------------------------------------------------------------------------
HOSTNAME_VAL="fotobox"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --hostname)
            [[ $# -ge 2 ]] || error "--hostname requires an argument"
            HOSTNAME_VAL="$2"
            shift 2
            ;;
        *)
            error "Unknown argument: $1"
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
info()  { echo "[INFO]  $*"; }
error() { echo "[ERROR] $*" >&2; exit 1; }

require_root() {
    [[ $EUID -eq 0 ]] || error "This script must be run as root (sudo bash install.sh)"
}

# ---------------------------------------------------------------------------
# 1. Hostname
# ---------------------------------------------------------------------------
configure_hostname() {
    info "Setting hostname to '$HOSTNAME_VAL'…"
    hostnamectl set-hostname "$HOSTNAME_VAL"
    # Update /etc/hosts so that the hostname resolves to loopback
    if grep -q '127\.0\.1\.1' /etc/hosts; then
        sed -i "s/^127\.0\.1\.1\s.*/127.0.1.1\t${HOSTNAME_VAL}/" /etc/hosts
    else
        echo -e "127.0.1.1\t${HOSTNAME_VAL}" >> /etc/hosts
    fi
}

# ---------------------------------------------------------------------------
# 2. Network
# ---------------------------------------------------------------------------
enable_network() {
    info "Enabling Wifi via NetworkManager…"
    nmcli radio wifi on
    nmcli device set wlan0 managed yes
}

# ---------------------------------------------------------------------------
# 3. System packages
# ---------------------------------------------------------------------------
install_packages() {
    info "Updating package lists…"
    apt-get update -qq

    info "Installing xorg and podman"
    apt-get install -y --no-install-recommends \
        xorg \
        xserver-xorg-video-fbturbo \
        unclutter \
        podman \
        xauth \
        vim
}

# ---------------------------------------------------------------------------
# 3b. Xorg fbturbo config (Raspberry Pi 5)
# ---------------------------------------------------------------------------
configure_xorg() {
    local cfg=/etc/X11/xorg.conf.d/10-fbturbo.conf
    if [[ -f $cfg ]]; then
        info "$cfg already exists — skipping."
        return
    fi
    info "Writing $cfg (fbturbo driver for Raspberry Pi 5)…"
    mkdir -p /etc/X11/xorg.conf.d
    cat > "$cfg" <<'EOF'
Section "OutputClass"
    Identifier "vc4"
    MatchDriver "vc4"
    Driver "modesetting"
    Option "PrimaryGPU" "true"
EndSection
EOF
    chmod 644 "$cfg"
}

# ---------------------------------------------------------------------------
# 4. Tailscale
# ---------------------------------------------------------------------------
install_tailscale() {
    info "Adding Tailscale apt repository…"
    mkdir -p --mode=0755 /usr/share/keyrings
    curl -fsSL https://pkgs.tailscale.com/stable/debian/trixie.noarmor.gpg \
        | tee /usr/share/keyrings/tailscale-archive-keyring.gpg >/dev/null
    curl -fsSL https://pkgs.tailscale.com/stable/debian/trixie.tailscale-keyring.list \
        | tee /etc/apt/sources.list.d/tailscale.list >/dev/null
    apt-get update -qq
    info "Installing Tailscale…"
    apt-get install -y --no-install-recommends tailscale
}

# ---------------------------------------------------------------------------
# 5. Configuration file
# ---------------------------------------------------------------------------
write_env_file() {
    local cfg=/etc/gophotobooth/env
    if [[ -f $cfg ]]; then
        info "Config file $cfg already exists — skipping."
        return
    fi
    info "Writing default config to $cfg…"
    mkdir -p /etc/gophotobooth
    cat > "$cfg" <<'EOF'
# gophotobooth site configuration
# Edit these values for your installation.

# Public URL used to build QR codes (no trailing slash)
GALLERY_BASE_URL=http://raspberrypi.local:8080

# GPIO pin for the trigger button (e.g. GPIO17). Leave empty to disable buzzer.
BUZZER_PIN=

# Storage directories (created automatically if missing)
PHOTOS_DIR=/var/lib/gophotobooth/photos
CACHE_DIR=/var/lib/gophotobooth/cache
EOF
    chmod 644 "$cfg"
}

# ---------------------------------------------------------------------------
# 6. Load container images
# ---------------------------------------------------------------------------
load_images() {
    local tar="$TARBALL_DIR/images.tar"
    [[ -f $tar ]] || error "images.tar not found at $tar"
    info "Loading container images (this may take a while)…"
    podman load < "$tar"
}

# ---------------------------------------------------------------------------
# 7. Systemd units
# ---------------------------------------------------------------------------
SYSTEMD_DIR=/etc/systemd/system

write_unit() {
    local name="$1" content="$2"
    info "Installing $SYSTEMD_DIR/$name"
    echo "$content" > "$SYSTEMD_DIR/$name"
    chmod 644 "$SYSTEMD_DIR/$name"
}

install_units() {
    # --- Xorg (display server) ---
    write_unit photobooth-xorg.service '[Unit]
Description=Xorg display server for gophotobooth
After=systemd-udev-settle.service
Wants=systemd-udev-settle.service
StartLimitIntervalSec=0

[Service]
ExecStart=/usr/bin/Xorg :0 -ac -nolisten tcp vt7
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- Unclutter (hide mouse cursor) ---
    write_unit photobooth-unclutter.service '[Unit]
Description=Hide mouse cursor for gophotobooth
After=photobooth-xorg.service
Requires=photobooth-xorg.service
StartLimitIntervalSec=0

[Service]
Environment=DISPLAY=:0
ExecStart=/usr/bin/unclutter -idle 0 -root
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- DSLR (gphoto2 camera bridge) ---
    write_unit photobooth-dslr.service '[Unit]
Description=gophotobooth DSLR service
After=network.target
StartLimitIntervalSec=0

[Service]
EnvironmentFile=/etc/gophotobooth/env
ExecStartPre=-/usr/bin/podman rm -f photobooth-dslr
ExecStartPre=/bin/mkdir -p ${PHOTOS_DIR} ${CACHE_DIR}
ExecStart=podman run --rm \
    --name photobooth-dslr \
    --network host \
    --privileged \
    --device /dev/bus/usb \
    gophotobooth-dslr
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- UI (Fyne fullscreen display) ---
    write_unit photobooth-ui.service '[Unit]
Description=gophotobooth UI service
After=photobooth-xorg.service
Requires=photobooth-xorg.service
StartLimitIntervalSec=0

[Service]
EnvironmentFile=/etc/gophotobooth/env
Environment=DISPLAY=:0
ExecStartPre=-/usr/bin/podman rm -f photobooth-ui
ExecStart=podman run --rm \
    --name photobooth-ui \
    --network host \
    -v /dev/dri:/dev/dri \
    --privileged \
    --uts=host \
    -e DISPLAY=:0 \
    -v /tmp/.X11-unix:/tmp/.X11-unix \
    gophotobooth-ui
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- Gallery (HTTP photo gallery) ---
    write_unit photobooth-gallery.service '[Unit]
Description=gophotobooth gallery service
After=network.target
StartLimitIntervalSec=0

[Service]
EnvironmentFile=/etc/gophotobooth/env
ExecStartPre=-/usr/bin/podman rm -f photobooth-gallery
ExecStartPre=/bin/mkdir -p ${PHOTOS_DIR} ${CACHE_DIR}
ExecStart=podman run --rm \
    --name photobooth-gallery \
    --network host \
    -v ${PHOTOS_DIR}:/photos:ro \
    -v ${CACHE_DIR}:/cache \
    gophotobooth-gallery \
    --directory /photos \
    --cache-dir /cache
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- Core (orchestrator) ---
    write_unit photobooth-core.service '[Unit]
Description=gophotobooth core service
After=photobooth-ui.service photobooth-dslr.service photobooth-gallery.service
Wants=photobooth-ui.service photobooth-dslr.service photobooth-gallery.service
StartLimitIntervalSec=0

[Service]
EnvironmentFile=/etc/gophotobooth/env
ExecStartPre=-/usr/bin/podman rm -f photobooth-core
ExecStartPre=/bin/mkdir -p ${PHOTOS_DIR}
ExecStart=podman run --rm \
    --name photobooth-core \
    --network host \
    -v ${PHOTOS_DIR}:/photos \
    gophotobooth-core \
    --directory /photos \
    --gallery-base-url ${GALLERY_BASE_URL}
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'

    # --- Buzzer (optional GPIO button) ---
    write_unit photobooth-buzzer.service '[Unit]
Description=gophotobooth buzzer service
After=photobooth-core.service
Wants=photobooth-core.service
ConditionPathExists=/etc/gophotobooth/env
StartLimitIntervalSec=0

[Service]
EnvironmentFile=/etc/gophotobooth/env
# Skip if BUZZER_PIN is unset or empty
ExecCondition=/bin/sh -c '"'"'test -n "${BUZZER_PIN}"'"'"'
ExecStartPre=-/usr/bin/podman rm -f photobooth-buzzer
ExecStart=podman run --rm \
    --name photobooth-buzzer \
    --network host \
    --privileged \
    --device /dev/gpiochip0 \
    gophotobooth-buzzer \
    --pin ${BUZZER_PIN}
Restart=always
RestartSec=1
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target'
}

# ---------------------------------------------------------------------------
# 8. Enable and start services
# ---------------------------------------------------------------------------
enable_services() {
    info "Reloading systemd…"
    systemctl daemon-reload

    local services=(
        photobooth-xorg
        photobooth-unclutter
        photobooth-dslr
        photobooth-ui
        photobooth-gallery
        photobooth-core
    )

    # Only enable buzzer if BUZZER_PIN is set in the env file
    if grep -qE '^BUZZER_PIN=.+' /etc/gophotobooth/env 2>/dev/null; then
        services+=(photobooth-buzzer)
    fi

    for svc in "${services[@]}"; do
        info "Enabling and starting $svc…"
        systemctl enable "$svc"
        systemctl start  "$svc" || true   # don't fail if camera/GPIO missing
    done
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
require_root
configure_hostname
enable_network
install_packages
configure_xorg
install_tailscale
write_env_file
load_images
install_units
enable_services

info ""
info "Installation complete."
info ""
info "Edit /etc/gophotobooth/env to set GALLERY_BASE_URL (and optionally BUZZER_PIN),"
info "then restart services:"
info "  sudo systemctl restart photobooth-core photobooth-buzzer"
info ""
info "View logs:"
info "  journalctl -u photobooth-core -f"
