// Command dslr runs the DSLR gRPC server, bridging a gphoto2 camera to Core.
// Requires CGO and libgphoto2 at build and runtime.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

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
	mu       sync.Mutex
	cam      *gphoto2.Camera
	shutdown chan<- error
}

func (s *server) close() {
	if s.cam != nil {
		s.cam.Close()
		s.cam = nil
	}
}

func (s *server) triggerShutdown(err error) {
	select {
	case s.shutdown <- err:
	default:
	}
}

func humanReadableError(phase string, err error) error {
	if strings.Contains(err.Error(), "init: Unknown model") {
		return fmt.Errorf("[%s] camera might be turned off, restarting:\n%w", phase, err)
	}
	return fmt.Errorf("[%s] unclassified error:\n%w", phase, err)
}

func (s *server) TakePhoto(_ context.Context, _ *emptypb.Empty) (*pb.TakePhotoResponse, error) {
	if !s.mu.TryLock() {
		return nil, status.Error(codes.Unavailable, "camera busy")
	}
	defer s.mu.Unlock()

	photo, err := s.cam.TakePhoto()
	if err != nil {
		// In case of an error, try to close the camera and create a new one.
		// If creating a new one also fails, we trigger shutdown to have a graceful restart.
		s.cam.Close()
		var newErr error
		s.cam, newErr = gphoto2.New()
		if newErr != nil {
			err = humanReadableError("Take photo", fmt.Errorf("camera failure (%w) and reconnect failure (%v):", err, newErr))
			s.triggerShutdown(err)
			return nil, status.Error(codes.Internal, err.Error())
		}
		// s.triggerShutdown(err)
		return nil, status.Errorf(codes.Internal, "failed to take a photo, camera reconnected. Please retry! (%v)", err)
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
				// TODO use human readable error
				return humanReadableError("Initialization", err)
			}

			shutdown := make(chan error, 1)
			srv := &server{cam: cam, shutdown: shutdown}
			defer srv.close()

			lis, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", listenAddr, err)
			}

			opts := []grpc.ServerOption{
				grpc.MaxSendMsgSize(32 * 1024 * 1024),
				grpc.MaxRecvMsgSize(32 * 1024 * 1024),
			}
			grpcSrv := grpc.NewServer(opts...)
			pb.RegisterDSLRServer(grpcSrv, srv)

			var cameraErr error
			go func() {
				err := <-shutdown
				cameraErr = err
				slog.Error("DSLR: camera error, stopping server", "err", err)
				grpcSrv.GracefulStop()
			}()

			slog.Info("DSLR gRPC server listening", "addr", listenAddr)
			if err := grpcSrv.Serve(lis); err != nil {
				return fmt.Errorf("gRPC server stopped: %w", err)
			}

			if cameraErr != nil {
				return fmt.Errorf("camera error: %w", cameraErr)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", constants.DSLRAddress, "gRPC listen address")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
