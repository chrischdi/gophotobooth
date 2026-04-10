// Command ui runs the photo booth display and its gRPC control server.
// It renders a fullscreen Fyne window and accepts display commands from Core.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"net"
	"os"

	qrcode "github.com/skip2/go-qrcode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	pb "github.com/chrischdi/gophotobooth/proto/ui"
)

//go:embed welcome.jpg
var welcomeJPEG []byte

// uiServer handles gRPC calls from Core and updates the Fyne window.
// All window mutations are dispatched to the Fyne main thread via fyne.DoAndWait.
//
// All layers are pre-built at startup and stored in mainStack. State transitions
// only toggle visibility and refresh in-place — SetContent is never called after init.
type uiServer struct {
	pb.UnimplementedUIServer
	window fyne.Window

	// pre-built layers (back to front in mainStack)
	photoCanvas      *canvas.Image        // bottom: current photo (or welcome image)
	dimLayer         *canvas.Rectangle    // semi-transparent dim during countdown/smile
	overlayCountdown [4]fyne.CanvasObject // indices 1, 2, 3 are valid
	overlaySmile     fyne.CanvasObject
	qrImage          *canvas.Image
	qrLayer          fyne.CanvasObject
	errorLabel       *widget.Label
	errorLayer       fyne.CanvasObject
	mainStack        fyne.CanvasObject // needed to force repaint before first ShowImage
	firstImageShown  bool
}

func bigRedText(s string, textSize float32) fyne.CanvasObject {
	t := canvas.NewText(s, color.RGBA{R: 255, A: 255})
	t.TextSize = textSize
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.Alignment = fyne.TextAlignCenter
	return container.NewCenter(t)
}

func buildCountdownOverlay(count int) fyne.CanvasObject {
	obj := container.NewCenter(container.NewVBox(
		container.NewCenter(bigRedText(fmt.Sprintf("%d", count), 250)),
		container.NewCenter(bigRedText("Blick nach unten!", 100)),
		bigRedText("▼", 150),
	))
	obj.Hide()
	return obj
}

func buildSmileOverlay() fyne.CanvasObject {
	obj := container.NewCenter(container.NewVBox(
		container.NewCenter(
			bigRedText("", 250),
			container.NewVBox(
				bigRedText("", 90),
				bigRedText("⌛", 150),
			),
			bigRedText("", 250),
		),
		container.NewCenter(bigRedText("Blick nach unten!", 100)),
		container.NewCenter(bigRedText("▼", 150)),
	))
	obj.Hide()
	return obj
}

func (s *uiServer) hideAllOverlays() {
	s.dimLayer.Hide()
	for i := 1; i <= 3; i++ {
		s.overlayCountdown[i].Hide()
	}
	s.overlaySmile.Hide()
	s.qrLayer.Hide()
	s.errorLayer.Hide()
}

func (s *uiServer) ShowCountdown(_ context.Context, req *pb.CountdownRequest) (*emptypb.Empty, error) {
	count := int(req.Count)
	if count < 1 || count > 3 {
		return nil, fmt.Errorf("invalid countdown value %d: must be 1, 2, or 3", count)
	}
	slog.Info("UI: ShowCountdown", "count", count)
	fyne.DoAndWait(func() {
		s.hideAllOverlays()
		s.dimLayer.Show()
		s.overlayCountdown[count].Show()
		if !s.firstImageShown {
			s.mainStack.Refresh()
		}
	})
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowSmile(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("UI: ShowSmile")
	fyne.DoAndWait(func() {
		s.hideAllOverlays()
		s.dimLayer.Show()
		s.overlaySmile.Show()
		if !s.firstImageShown {
			s.mainStack.Refresh()
		}
	})
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowImage(_ context.Context, req *pb.ShowImageRequest) (*emptypb.Empty, error) {
	img, _, err := image.Decode(bytes.NewReader(req.ImageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	slog.Info("UI: ShowImage", "bytes", len(req.ImageBytes), "photo_url", req.PhotoUrl)

	// Generate QR code outside the UI thread.
	var qrImg image.Image
	if req.PhotoUrl != "" {
		qrc, err := qrcode.New(req.PhotoUrl, qrcode.Medium)
		if err != nil {
			slog.Warn("UI: failed to generate QR code", "err", err)
		} else {
			qrc.DisableBorder = true
			qrPNG, err := qrc.PNG(256)
			if err != nil {
				slog.Warn("UI: failed to encode QR PNG", "err", err)
			} else {
				decoded, _, err := image.Decode(bytes.NewReader(qrPNG))
				if err != nil {
					slog.Warn("UI: failed to decode QR PNG", "err", err)
				} else {
					qrImg = addWhiteBorder(decoded, 5)
				}
			}
		}
	}

	fyne.DoAndWait(func() {
		s.hideAllOverlays()
		s.photoCanvas.Image = img
		s.photoCanvas.Refresh()
		s.firstImageShown = true
		if qrImg != nil {
			s.qrImage.Image = qrImg
			s.qrImage.Refresh()
			s.qrLayer.Show()
		}
	})
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowError(_ context.Context, req *pb.ShowErrorRequest) (*emptypb.Empty, error) {
	slog.Info("UI: ShowError", "message", req.Message)
	fyne.DoAndWait(func() {
		s.hideAllOverlays()
		s.errorLabel.SetText("Error: " + req.Message)
		s.errorLayer.Show()
	})
	return &emptypb.Empty{}, nil
}

func main() {
	var listenAddr string

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Photo booth display gRPC server",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := app.New()
			a.Settings().SetTheme(&darkTheme{})

			win := a.NewWindow("gophotobooth")
			win.SetMaster()
			win.SetFullScreen(true)

			// Build welcome image as initial photo layer.
			welcomeImg, _, _ := image.Decode(bytes.NewReader(welcomeJPEG))
			photoCanvas := canvas.NewImageFromImage(welcomeImg)
			photoCanvas.FillMode = canvas.ImageFillContain
			photoCanvas.ScaleMode = canvas.ImageScaleSmooth

			dimLayer := canvas.NewRectangle(color.RGBA{A: 125})
			dimLayer.Hide()

			// Pre-build countdown overlays.
			var overlayCountdown [4]fyne.CanvasObject
			for i := 1; i <= 3; i++ {
				overlayCountdown[i] = buildCountdownOverlay(i)
			}

			overlaySmile := buildSmileOverlay()

			// Pre-build QR layer with a placeholder image (updated on ShowImage).
			qrImage := canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 256, 256)))
			qrImage.FillMode = canvas.ImageFillOriginal
			qrImage.SetMinSize(fyne.NewSize(200, 200))
			qrLayer := container.NewBorder(nil,
				container.NewHBox(layout.NewSpacer(), qrImage),
				nil, nil)
			qrLayer.Hide()

			// Pre-build error layer.
			errorLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
			errorLayer := container.NewCenter(errorLabel)
			errorLayer.Hide()

			mainStack := container.NewStack(
				photoCanvas,
				dimLayer,
				overlayCountdown[1],
				overlayCountdown[2],
				overlayCountdown[3],
				overlaySmile,
				qrLayer,
				errorLayer,
			)
			win.SetContent(mainStack)

			srv := &uiServer{
				window:           win,
				photoCanvas:      photoCanvas,
				dimLayer:         dimLayer,
				overlayCountdown: overlayCountdown,
				overlaySmile:     overlaySmile,
				qrImage:          qrImage,
				qrLayer:          qrLayer,
				errorLabel:       errorLabel,
				errorLayer:       errorLayer,
				mainStack:        mainStack,
			}

			lis, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", listenAddr, err)
			}

			opts := []grpc.ServerOption{
				grpc.MaxSendMsgSize(32 * 1024 * 1024),
				grpc.MaxRecvMsgSize(32 * 1024 * 1024),
			}
			grpcSrv := grpc.NewServer(opts...)
			pb.RegisterUIServer(grpcSrv, srv)

			go func() {
				slog.Info("UI gRPC server listening", "addr", listenAddr)
				if err := grpcSrv.Serve(lis); err != nil {
					slog.Error("gRPC server stopped", "err", err)
					os.Exit(1)
				}
			}()

			win.ShowAndRun()
			return nil
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", constants.UIAddress, "gRPC listen address")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
