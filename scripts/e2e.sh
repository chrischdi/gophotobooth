#!/usr/bin/env bash
# e2e.sh - End-to-end test for gophotobooth using real CGO-free binaries.
#
# Starts stub services (ui-serve-dummy, dslr-serve-dummy), core, and gallery,
# triggers the photo workflow, and asserts observable outcomes via HTTP and
# the filesystem.
#
# Prerequisites: make build (or CGO_ENABLED=0 go build ./cmd/...) must have
# been run first so that bin/{core,ctl,gallery} exist.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$REPO_ROOT/bin"

PHOTO_DIR=$(mktemp -d)
CACHE_DIR=$(mktemp -d)

PASS=0
FAIL=0

# ---- helpers ----

pass() { echo "PASS: $1"; ((PASS++)); }
fail() { echo "FAIL: $1"; ((FAIL++)); }

assert_eq() {
    local desc="$1" got="$2" want="$3"
    if [[ "$got" == "$want" ]]; then pass "$desc"; else fail "$desc (got $got, want $want)"; fi
}

assert_contains() {
    local desc="$1" haystack="$2" needle="$3"
    if echo "$haystack" | grep -q "$needle"; then pass "$desc"; else fail "$desc (needle '$needle' not found)"; fi
}

wait_tcp() {
    local host="$1" port="$2" timeout=20
    local start=$SECONDS
    while ! (echo > /dev/tcp/"$host"/"$port") 2>/dev/null; do
        if (( SECONDS - start >= timeout )); then
            echo "ERROR: $host:$port did not become ready within ${timeout}s" >&2
            exit 1
        fi
        sleep 0.1
    done
}

cleanup() {
    kill "${DSLR_PID:-}" "${UI_PID:-}" "${CORE_PID:-}" "${GALLERY_PID:-}" 2>/dev/null || true
    rm -rf "$PHOTO_DIR" "$CACHE_DIR"
}
trap cleanup EXIT INT TERM

# ---- check prerequisites ----

for bin in core ctl gallery; do
    if [[ ! -x "$BIN/$bin" ]]; then
        echo "ERROR: $BIN/$bin not found — run 'make build' first" >&2
        exit 1
    fi
done

for tool in curl; do
    if ! command -v "$tool" &>/dev/null; then
        echo "ERROR: $tool is required but not installed" >&2
        exit 1
    fi
done

echo "==> gophotobooth e2e test"
echo "    PHOTO_DIR=$PHOTO_DIR"
echo ""

# ---- start services ----

"$BIN/ctl" dslr-serve-dummy --dslr-address localhost:50002 &
DSLR_PID=$!

"$BIN/ctl" ui-serve-dummy --ui-address localhost:50001 &
UI_PID=$!

"$BIN/core" \
    --listen localhost:50000 \
    --ui-address localhost:50001 \
    --dslr-address localhost:50002 \
    --directory "$PHOTO_DIR" \
    --post-capture-seconds 0 \
    --auto-trigger-minutes 999 &
CORE_PID=$!

"$BIN/gallery" \
    --listen localhost:8080 \
    --directory "$PHOTO_DIR" \
    --cache-dir "$CACHE_DIR" &
GALLERY_PID=$!

echo "==> Waiting for services to be ready..."
wait_tcp localhost 50002
wait_tcp localhost 50001
wait_tcp localhost 50000

# ---- test: trigger workflow ----

echo ""
echo "==> Triggering workflow..."
"$BIN/ctl" core-trigger --core-address localhost:50000

# Countdown takes ~3s (post-capture 0s); add buffer for DSLR 500ms delay
sleep 6

PHOTO_COUNT=$(ls "$PHOTO_DIR"/*.jpg 2>/dev/null | wc -l || echo 0)
assert_eq "one photo written after trigger" "$PHOTO_COUNT" "1"

# Wait for gallery to detect the first photo and start serving
wait_tcp localhost 8080

# ---- test: robots.txt ----

ROBOTS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/robots.txt)
assert_eq "robots.txt returns 200" "$ROBOTS_STATUS" "200"

ROBOTS_BODY=$(curl -s http://localhost:8080/robots.txt)
assert_contains "robots.txt contains Disallow" "$ROBOTS_BODY" "Disallow"

# ---- test: gallery page ----

OLDEST=$(ls "$PHOTO_DIR"/*.jpg | sort | head -1 | xargs -I{} basename {})
PREFIX=$("$BIN/ctl" gallery-hash "$OLDEST")
GALLERY_URL="http://localhost:8080/g/${PREFIX}/"

GALLERY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GALLERY_URL")
assert_eq "gallery page at correct hash returns 200" "$GALLERY_STATUS" "200"

WRONG_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8080/g/wronghash1234567/")
assert_eq "wrong gallery hash returns 404" "$WRONG_STATUS" "404"

# ---- test: photo endpoint ----

PHOTO_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8080/photo/$OLDEST")
assert_eq "full photo served at /photo/<name>" "$PHOTO_STATUS" "200"

# ---- test: unknown path returns 404 ----

UNKNOWN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:8080/unknown")
assert_eq "unknown path returns 404" "$UNKNOWN_STATUS" "404"

# ---- test: second trigger after first completes ----

echo ""
echo "==> Triggering second workflow..."
"$BIN/ctl" core-trigger --core-address localhost:50000
sleep 6

PHOTO_COUNT=$(ls "$PHOTO_DIR"/*.jpg 2>/dev/null | wc -l || echo 0)
if (( PHOTO_COUNT >= 2 )); then
    pass "second trigger produces a second photo (total $PHOTO_COUNT)"
else
    fail "expected >=2 photos after second trigger, got $PHOTO_COUNT"
fi

# ---- summary ----

echo ""
echo "=============================="
echo "Results: $PASS passed, $FAIL failed"
echo "=============================="

if (( FAIL > 0 )); then
    exit 1
fi
