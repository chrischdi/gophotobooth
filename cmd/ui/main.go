// Command ui runs the photo booth display and its gRPC control server.
// It renders a fullscreen Fyne window and accepts display commands from Core.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"net"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	pb "github.com/chrischdi/gophotobooth/proto/ui"
)

// uiServer handles gRPC calls from Core and updates the Fyne window.
// All window mutations are dispatched to the Fyne main thread via fyne.DoAndWait.
type uiServer struct {
	pb.UnimplementedUIServer
	window fyne.Window
}

func (s *uiServer) setContent(obj fyne.CanvasObject) {
	fyne.DoAndWait(func() {
		s.window.SetContent(obj)
	})
}

func (s *uiServer) ShowIdle(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("UI: ShowIdle")
	s.setContent(container.NewCenter(
		widget.NewLabelWithStyle("Welcome", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	))
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowCountdown(_ context.Context, req *pb.CountdownRequest) (*emptypb.Empty, error) {
	slog.Info("UI: ShowCountdown", "count", req.Count)
	s.setContent(container.NewCenter(
		widget.NewRichTextFromMarkdown(fmt.Sprintf("# %d", req.Count)),
	))
	return &emptypb.Empty{}, nil
}

func (s *uiServer) ShowSmile(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("UI: ShowSmile")
	s.setContent(container.NewCenter(
		widget.NewRichTextFromMarkdown("# Smile!"),
	))
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
	slog.Info("UI: ShowImage", "bytes", len(req.ImageBytes))
	s.setContent(fyneImg)
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
			a.Settings().SetTheme(theme.DarkTheme())

			win := a.NewWindow("gophotobooth")
			win.SetFullScreen(true)
			win.SetMaster()

			srv := &uiServer{window: win}
			win.SetContent(canvas.NewRectangle(colorBlack()))

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
