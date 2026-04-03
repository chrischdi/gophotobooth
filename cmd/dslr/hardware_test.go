//go:build hardware

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"testing"

	"github.com/chrischdi/gophotobooth/pkg/camera/gphoto2"
)

// TestHardware_DSLRTakePhoto requires a gphoto2-compatible camera connected via USB.
//
// Run with:
//
//	CGO_ENABLED=1 go test -tags hardware -run TestHardware_DSLRTakePhoto ./cmd/dslr/ -v
func TestHardware_DSLRTakePhoto(t *testing.T) {
	fmt.Fprintln(os.Stderr, "\n>>> Ensure camera is connected and powered on <<<")
	fmt.Fprint(os.Stderr, ">>> Press Enter to proceed (or Ctrl+C to abort): ")
	bufio.NewReader(os.Stdin).ReadString('\n') //nolint:errcheck

	cam, err := gphoto2.New()
	if err != nil {
		t.Skipf("gphoto2 init failed (no camera?): %v", err)
	}

	photo, err := cam.TakePhoto()
	if err != nil {
		t.Fatalf("TakePhoto: %v", err)
	}

	if len(photo.Bytes) == 0 {
		t.Fatal("TakePhoto returned empty bytes")
	}

	img, format, err := image.Decode(bytes.NewReader(photo.Bytes))
	if err != nil {
		t.Fatalf("failed to decode photo as image: %v", err)
	}

	t.Logf("Captured: format=%s size=%dx%d filename=%s len=%d bytes",
		format, img.Bounds().Dx(), img.Bounds().Dy(), photo.Filename, len(photo.Bytes))
}
