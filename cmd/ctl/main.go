// Command ctl is the photo booth admin CLI.
// It allows triggering a workflow, sending display commands, and running stub
// servers for isolated testing.
//
// Commands are prefixed with the target service name:
//
//	core-trigger        Trigger a photo workflow
//	core-serve-dummy    Stub Core server (for testing Buzzer in isolation)
//	ui-show-idle        Send ShowIdle to the UI
//	ui-show-image       Send a JPEG file to the UI
//	ui-serve-dummy      Stub UI server (for testing Core without a display)
//	dslr-serve-dummy    Dummy DSLR server returning generated timestamp images
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	"github.com/chrischdi/gophotobooth/pkg/camera/dummy"
	pbcore "github.com/chrischdi/gophotobooth/proto/core"
	pbdslr "github.com/chrischdi/gophotobooth/proto/dslr"
	pbui "github.com/chrischdi/gophotobooth/proto/ui"
)

func dialCore(addr string) (pbcore.CoreClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return pbcore.NewCoreClient(conn), conn, nil
}

func dialUI(addr string) (pbui.UIClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return pbui.NewUIClient(conn), conn, nil
}

// --- stub servers for isolated testing ---

type stubCoreServer struct {
	pbcore.UnimplementedCoreServer
}

func (s *stubCoreServer) Trigger(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("stub Core: Trigger received")
	return &emptypb.Empty{}, nil
}

type stubUIServer struct {
	pbui.UnimplementedUIServer
}

func (s *stubUIServer) ShowIdle(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("stub UI: ShowIdle")
	return &emptypb.Empty{}, nil
}
func (s *stubUIServer) ShowCountdown(_ context.Context, req *pbui.CountdownRequest) (*emptypb.Empty, error) {
	slog.Info("stub UI: ShowCountdown", "count", req.Count)
	return &emptypb.Empty{}, nil
}
func (s *stubUIServer) ShowSmile(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	slog.Info("stub UI: ShowSmile")
	return &emptypb.Empty{}, nil
}
func (s *stubUIServer) ShowImage(_ context.Context, req *pbui.ShowImageRequest) (*emptypb.Empty, error) {
	slog.Info("stub UI: ShowImage", "bytes", len(req.ImageBytes))
	return &emptypb.Empty{}, nil
}
func (s *stubUIServer) ShowError(_ context.Context, req *pbui.ShowErrorRequest) (*emptypb.Empty, error) {
	slog.Info("stub UI: ShowError", "message", req.Message)
	return &emptypb.Empty{}, nil
}

type dummyDSLRServer struct {
	pbdslr.UnimplementedDSLRServer
	cam *dummy.Camera
}

func (s *dummyDSLRServer) TakePhoto(_ context.Context, _ *emptypb.Empty) (*pbdslr.TakePhotoResponse, error) {
	photo, err := s.cam.TakePhoto()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "dummy take photo: %v", err)
	}
	slog.Info("dummy DSLR: TakePhoto", "filename", photo.Filename)
	return &pbdslr.TakePhotoResponse{
		ImageBytes: photo.Bytes,
		Filename:   photo.Filename,
	}, nil
}

func main() {
	var coreAddr, uiAddr, dslrAddr string

	root := &cobra.Command{
		Use:   "ctl",
		Short: "Photo booth admin CLI",
	}

	root.PersistentFlags().StringVar(&coreAddr, "core-address", constants.CoreAddress, "Core service gRPC address")
	root.PersistentFlags().StringVar(&uiAddr, "ui-address", constants.UIAddress, "UI service gRPC address")
	root.PersistentFlags().StringVar(&dslrAddr, "dslr-address", constants.DSLRAddress, "DSLR service gRPC address")

	root.AddCommand(&cobra.Command{
		Use:   "core-trigger",
		Short: "Trigger a photo workflow on the Core service",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := dialCore(coreAddr)
			if err != nil {
				return err
			}
			defer conn.Close()
			if _, err = client.Trigger(context.Background(), &emptypb.Empty{}); err != nil {
				return fmt.Errorf("trigger: %w", err)
			}
			fmt.Println("workflow triggered")
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "core-serve-dummy",
		Short: "Run a stub Core gRPC server (for testing Buzzer in isolation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			lis, err := net.Listen("tcp", coreAddr)
			if err != nil {
				return err
			}
			srv := grpc.NewServer()
			pbcore.RegisterCoreServer(srv, &stubCoreServer{})
			slog.Info("stub Core listening", "addr", coreAddr)
			return srv.Serve(lis)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "ui-show-idle",
		Short: "Send ShowIdle to the UI service",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close()
			_, err = client.ShowIdle(context.Background(), &emptypb.Empty{})
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "ui-show-image <file>",
		Short: "Display a JPEG file on the UI service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close()
			_, err = client.ShowImage(context.Background(), &pbui.ShowImageRequest{
				ImageBytes: data,
				Format:     "image/jpeg",
			}, grpc.MaxCallSendMsgSize(32*1024*1024))
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "ui-serve-dummy",
		Short: "Run a stub UI gRPC server (for testing Core without a display)",
		RunE: func(cmd *cobra.Command, args []string) error {
			lis, err := net.Listen("tcp", uiAddr)
			if err != nil {
				return err
			}
			srv := grpc.NewServer(grpc.MaxRecvMsgSize(32 * 1024 * 1024))
			pbui.RegisterUIServer(srv, &stubUIServer{})
			slog.Info("stub UI listening", "addr", uiAddr)
			return srv.Serve(lis)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "dslr-serve-dummy",
		Short: "Run a dummy DSLR gRPC server returning generated timestamp images",
		RunE: func(cmd *cobra.Command, args []string) error {
			lis, err := net.Listen("tcp", dslrAddr)
			if err != nil {
				return err
			}
			srv := grpc.NewServer(grpc.MaxSendMsgSize(32 * 1024 * 1024))
			pbdslr.RegisterDSLRServer(srv, &dummyDSLRServer{cam: &dummy.Camera{}})
			slog.Info("dummy DSLR listening", "addr", dslrAddr)
			return srv.Serve(lis)
		},
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
