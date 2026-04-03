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
type uiServer struct {
	pb.UnimplementedUIServer
	window    fyne.Window
	lastPhoto fyne.CanvasObject // photo only, used as countdown background
	lastQR    fyne.CanvasObject // QR overlay only (nil when no QR), placed above the dim layer
}

func (s *uiServer) setContent(obj fyne.CanvasObject) {
	fyne.DoAndWait(func() {
		s.window.SetContent(obj)
	})
}

func bigRedText(s string, textSize float32) fyne.CanvasObject {
	t := canvas.NewText(s, color.RGBA{R: 255, A: 255})
	t.TextSize = textSize
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.Alignment = fyne.TextAlignCenter
	return container.NewCenter(t)
}

func (s *uiServer) ShowCountdown(_ context.Context, req *pb.CountdownRequest) (*emptypb.Empty, error) {
	slog.Info("UI: ShowCountdown", "count", req.Count)

	t := canvas.NewText(fmt.Sprintf("%d", req.Count), color.RGBA{R: 255, A: 255})
	t.TextSize = 250
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.Alignment = fyne.TextAlignCenter

	dim := canvas.NewRectangle(color.RGBA{A: 125})

	layers := []fyne.CanvasObject{
		s.lastPhoto,
		dim,
		container.NewCenter(container.NewVBox(
			container.NewCenter(bigRedText(fmt.Sprintf("%d", req.Count), 250)),
			container.NewCenter(bigRedText("Blick nach unten!", 100)),
			bigRedText("▼", 150),
		)),
	}
	if s.lastQR != nil {
		layers = append(layers, s.lastQR)
	}
	s.setContent(container.NewStack(layers...))
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowSmile(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("UI: ShowSmile")

	dim := canvas.NewRectangle(color.RGBA{A: 125})
	layers := []fyne.CanvasObject{
		s.lastPhoto,
		dim,
		container.NewCenter(container.NewVBox(
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
		)),
	}
	if s.lastQR != nil {
		layers = append(layers, s.lastQR)
	}
	s.setContent(container.NewStack(layers...))
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowImage(_ context.Context, req *pb.ShowImageRequest) (*emptypb.Empty, error) {
	img, _, err := image.Decode(bytes.NewReader(req.ImageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	fyneImg := canvas.NewImageFromImage(img)
	fyneImg.FillMode = canvas.ImageFillContain
	fyneImg.ScaleMode = canvas.ImageScaleSmooth
	slog.Info("UI: ShowImage", "bytes", len(req.ImageBytes), "photo_url", req.PhotoUrl)

	s.lastPhoto = fyneImg
	s.lastQR = nil

	if req.PhotoUrl != "" {
		qrc, err := qrcode.New(req.PhotoUrl, qrcode.Medium)
		if err != nil {
			slog.Warn("UI: failed to generate QR code", "err", err)
			qrc = nil
		}
		var qrPNG []byte
		if qrc != nil {
			qrc.DisableBorder = true
			qrPNG, err = qrc.PNG(256)
		}
		if err != nil {
			slog.Warn("UI: failed to generate QR code", "err", err)
		} else {
			qrImg, _, err := image.Decode(bytes.NewReader(qrPNG))
			if err != nil {
				slog.Warn("UI: failed to decode QR PNG", "err", err)
			} else {
				qrImg = addWhiteBorder(qrImg, 5)
				qr := canvas.NewImageFromImage(qrImg)
				qr.FillMode = canvas.ImageFillOriginal
				qr.SetMinSize(fyne.NewSize(200, 200))
				s.lastQR = container.NewBorder(nil,
					container.NewHBox(layout.NewSpacer(), qr),
					nil, nil)
			}
		}
	}

	var content fyne.CanvasObject = fyneImg
	if s.lastQR != nil {
		content = container.NewStack(fyneImg, s.lastQR)
	}
	s.setContent(content)
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowError(_ context.Context, req *pb.ShowErrorRequest) (*emptypb.Empty, error) {
	slog.Info("UI: ShowError", "message", req.Message)
	s.setContent(container.NewCenter(
		widget.NewLabelWithStyle("Error: "+req.Message, fyne.TextAlignCenter, fyne.TextStyle{}),
	))
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

			srv := &uiServer{window: win, lastPhoto: canvas.NewRectangle(colorBlack())}
			welcomeImg, _, _ := image.Decode(bytes.NewReader(welcomeJPEG))
			fyneWelcome := canvas.NewImageFromImage(welcomeImg)
			fyneWelcome.FillMode = canvas.ImageFillContain
			fyneWelcome.ScaleMode = canvas.ImageScaleSmooth
			win.SetContent(fyneWelcome)

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
