# gophotobooth

A photo booth application for Raspberry Pi 5, built as Go microservices communicating over gRPC and packaged in containers.

## Architecture

Six services in a single Go module (`github.com/chrischdi/gophotobooth`):

| Service | Binary | Description |
|---------|--------|-------------|
| Core | `cmd/core` | Orchestrator. Owns the photo workflow and auto-trigger timer. |
| UI | `cmd/ui` | Fullscreen Fyne display. Receives display commands from Core via gRPC. |
| DSLR | `cmd/dslr` | gphoto2 camera bridge. Called by Core to take a photo. |
| Buzzer | `cmd/buzzer` | Listens for a GPIO button press and calls `Core.Trigger`. |
| Gallery | `cmd/gallery` | HTTP gallery served at an obscure SHA-256-derived URL. |
| ctl | `cmd/ctl` | Admin CLI and stub servers for local testing. |

Proto definitions live in `proto/{core,ui,dslr}/`. Generated `*.pb.go` files are **not** committed — run `make generate` to (re)create them.

### Photo workflow (Core)

```
Startup        → UI.ShowIdle()
Trigger()      → ShowCountdown(3) → sleep 1s
               → ShowCountdown(2) → sleep 1s
               → ShowCountdown(1) → sleep 1s
               → ShowSmile()
               → DSLR.TakePhoto() → write JPEG to --directory
               → UI.ShowImage()
               → sleep --post-capture-seconds   ← blocks next trigger
               → unlock (UI stays on last photo)
```

At most one workflow runs at a time (`sync.Mutex.TryLock`). A concurrent `Trigger` call returns `codes.AlreadyExists`.

### CGO boundary

Only two binaries require CGO:

- `cmd/dslr` — links against `libgphoto2`
- `cmd/ui` — links against OpenGL/X11 for Fyne

All other binaries (`core`, `buzzer`, `gallery`, `ctl`) build with `CGO_ENABLED=0` and produce fully static binaries.

## Build & development

### Prerequisites

- Go 1.26.1
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (for proto regeneration)
- `libgphoto2-dev` and X11/OpenGL headers (for CGO binaries, only needed on the build host)
- `golangci-lint`

Install the protoc plugins:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Common commands

```bash
make generate      # Regenerate *.pb.go from proto files
make build         # Build all binaries to bin/ (host arch)
make build-arm64   # Cross-compile for Raspberry Pi 5 (arm64)
make test          # go test ./...
make vet           # go vet ./...
make image         # Build all container images via podman
```

CGO-free services can always be built with `CGO_ENABLED=0 go build ./cmd/<service>`.

### Proto changes

Edit the `.proto` file, then run:

```bash
make generate
# or
./proto/regen.sh
```

Never edit `*.pb.go` files by hand.

## Testing locally without hardware

Use `ctl` stub servers to replace real hardware:

```bash
# 1. Stub UI (no display needed)
./bin/ctl ui-serve-dummy

# 2. Dummy DSLR (generates timestamp images)
./bin/ctl dslr-serve-dummy

# 3. Core
./bin/core --directory /tmp/photos

# 4. Trigger a workflow
./bin/ctl core-trigger
```

### ctl commands

| Command | Target | Description |
|---------|--------|-------------|
| `core-trigger` | Core | Trigger a photo workflow |
| `core-serve-dummy` | Core | Stub Core server (buzzer isolation) |
| `ui-show-idle` | UI | Send ShowIdle |
| `ui-show-image <file>` | UI | Send a JPEG to the display |
| `ui-serve-dummy` | UI | Stub UI server (no display needed) |
| `dslr-serve-dummy` | DSLR | Dummy DSLR returning timestamp images |

## Default ports

| Service | Address |
|---------|---------|
| Core | `localhost:50000` |
| UI | `localhost:50001` |
| DSLR | `localhost:50002` |
| Gallery HTTP | `0.0.0.0:8080` |

Defined in `internal/constants/constants.go`.

## Public access via Cloudflare Tunnel

The gallery is exposed publicly through a **Cloudflare Tunnel** (no open ports on the Pi):

- `cloudflare/cloudflared:2026.1.2` runs as `photobooth-cloudflared.service`
- Enabled by setting `CLOUDFLARED_TOKEN` in `/etc/gophotobooth/env`
- The tunnel token is obtained from **Cloudflare Zero Trust → Networks → Tunnels**
- In the tunnel's Public Hostnames tab, configure: `photos.example.com` → `http://localhost:8080`
- Set SSL/TLS mode to **Full** in the Cloudflare dashboard (tunnel leg is already encrypted)
- Cloudflare Universal SSL provides the public HTTPS certificate automatically — no Let's Encrypt needed
- `GALLERY_BASE_URL` in the env file should be set to the public hostname (e.g. `https://photos.example.com`) so Core builds correct QR codes

## Linting

```bash
golangci-lint run ./...
```

CI runs `golangci-lint` on every pull request. Fix all lint errors before merging.

## CI (GitHub Actions)

Every PR must pass:

1. `make generate` — verify proto stubs are up to date (commit the diff if not)
2. `go vet ./...`
3. `golangci-lint run ./...`
4. `CGO_ENABLED=0 go build ./cmd/core ./cmd/buzzer ./cmd/gallery ./cmd/ctl`
5. `make test`

CGO binaries (`cmd/dslr`, `cmd/ui`) are only built in CI if the runner has the required system libraries installed.

## Key conventions

- **No `ShowIdle` after the first photo.** The UI stays on the last captured photo between workflows. `ShowIdle` is called only once, at Core startup.
- **Core is the sole writer of the photos directory.** It receives bytes from DSLR via gRPC, writes to disk, then forwards bytes to UI. Gallery mounts the directory read-only.
- **gRPC message size limit is 32 MB** on DSLR (send) and UI (receive) to accommodate large JPEG files. Default gRPC max is 4 MB — do not remove these overrides.
- **Dummy camera** (`pkg/camera/dummy`) uses pure Go + embedded `gobold` font to render a timestamp image. No CGO, no external tools.
- **GPIO buzzer** (`pkg/buzzer/gpio`) uses periph.io and blocks on a falling edge. The pin name (e.g. `GPIO17`) is configurable via `--pin`.
- **Gallery URL** is `/g/{first-16-hex-chars-of-sha256-of-oldest-photo-filename}/`. `robots.txt` disallows all crawlers. Unknown paths return 404.
