// Command ctl is the photo booth admin CLI.
// It allows triggering a workflow, sending display commands, and running stub
// servers for isolated testing.
//
// Commands are prefixed with the target service name:
//
//	core-trigger        Trigger a photo workflow
//	core-serve-dummy    Stub Core server (for testing Buzzer in isolation)
//	ui-show-countdown   Send ShowCountdown to the UI
//	ui-show-smile       Send ShowSmile to the UI
//	ui-show-image       Send a JPEG file to the UI
//	ui-show-error       Send ShowError to the UI
//	ui-serve-dummy      Stub UI server (for testing Core without a display)
//	dslr-take-photo     Call TakePhoto on the DSLR service and save the result
//	dslr-serve-dummy    Dummy DSLR server returning generated timestamp images
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	"github.com/chrischdi/gophotobooth/internal/galleryurl"
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

func dialDSLR(addr string) (pbdslr.DSLRClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32*1024*1024)),
	)
	if err != nil {
		return nil, nil, err
	}
	return pbdslr.NewDSLRClient(conn), conn, nil
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
	time.Sleep(time.Millisecond * 500)
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
			defer conn.Close() //nolint:errcheck
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
		Use:   "ui-show-countdown <count>",
		Short: "Send ShowCountdown to the UI service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("count must be an integer: %w", err)
			}
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close() //nolint:errcheck
			_, err = client.ShowCountdown(context.Background(), &pbui.CountdownRequest{Count: int32(n)})
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "ui-show-smile",
		Short: "Send ShowSmile to the UI service",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close() //nolint:errcheck
			_, err = client.ShowSmile(context.Background(), &emptypb.Empty{})
			return err
		},
	})

	showImageCmd := &cobra.Command{
		Use:   "ui-show-image <file>",
		Short: "Display a JPEG file on the UI service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			photoURL, _ := cmd.Flags().GetString("photo-url")
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close() //nolint:errcheck
			_, err = client.ShowImage(context.Background(), &pbui.ShowImageRequest{
				ImageBytes: data,
				Format:     "image/jpeg",
				PhotoUrl:   photoURL,
			}, grpc.MaxCallSendMsgSize(32*1024*1024))
			return err
		},
	}
	showImageCmd.Flags().String("photo-url", "", "gallery deep-link URL to display as a QR code")
	root.AddCommand(showImageCmd)

	root.AddCommand(&cobra.Command{
		Use:   "ui-show-error <message>",
		Short: "Send ShowError to the UI service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := dialUI(uiAddr)
			if err != nil {
				return err
			}
			defer conn.Close() //nolint:errcheck
			_, err = client.ShowError(context.Background(), &pbui.ShowErrorRequest{Message: args[0]})
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

	dslrTakePhotoCmd := &cobra.Command{
		Use:   "dslr-take-photo",
		Short: "Call TakePhoto on the DSLR service and print stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			client, conn, err := dialDSLR(dslrAddr)
			if err != nil {
				return err
			}
			defer conn.Close() //nolint:errcheck
			resp, err := client.TakePhoto(context.Background(), &emptypb.Empty{})
			if err != nil {
				return fmt.Errorf("TakePhoto: %w", err)
			}
			fmt.Printf("filename: %s\nsize:     %d bytes\n", resp.Filename, len(resp.ImageBytes))
			if output != "" {
				if err := os.WriteFile(output, resp.ImageBytes, 0644); err != nil {
					return fmt.Errorf("write %s: %w", output, err)
				}
				fmt.Printf("saved:    %s\n", output)
			}
			return nil
		},
	}
	dslrTakePhotoCmd.Flags().String("output", "", "save the photo to this file path")
	root.AddCommand(dslrTakePhotoCmd)

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

	root.AddCommand(&cobra.Command{
		Use:   "gallery-hash <name>",
		Short: "Print the gallery URL hash prefix for a photo filename",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(galleryurl.HashPrefix(args[0]))
			return nil
		},
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
