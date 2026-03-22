// Command dslr runs the DSLR gRPC server, bridging a gphoto2 camera to Core.
// Requires CGO and libgphoto2 at build and runtime.
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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	"github.com/chrischdi/gophotobooth/pkg/camera/gphoto2"
	pb "github.com/chrischdi/gophotobooth/proto/dslr"
)

type server struct {
	pb.UnimplementedDSLRServer
	cam *gphoto2.Camera
}

func (s *server) TakePhoto(_ context.Context, _ *emptypb.Empty) (*pb.TakePhotoResponse, error) {
	photo, err := s.cam.TakePhoto()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "take photo: %v", err)
	}
	return &pb.TakePhotoResponse{
		ImageBytes: photo.Bytes,
		Filename:   photo.Filename,
	}, nil
}

func main() {
	var listenAddr string

	cmd := &cobra.Command{
		Use:   "dslr",
		Short: "DSLR gRPC camera bridge (gphoto2)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cam, err := gphoto2.New()
			if err != nil {
				return fmt.Errorf("gphoto2 init: %w", err)
			}
			defer cam.Close()

			lis, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", listenAddr, err)
			}

			opts := []grpc.ServerOption{
				grpc.MaxSendMsgSize(32 * 1024 * 1024),
				grpc.MaxRecvMsgSize(32 * 1024 * 1024),
			}
			srv := grpc.NewServer(opts...)
			pb.RegisterDSLRServer(srv, &server{cam: cam})

			slog.Info("DSLR gRPC server listening", "addr", listenAddr)
			return srv.Serve(lis)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", constants.DSLRAddress, "gRPC listen address")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
