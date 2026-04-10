// Command core is the photo booth orchestrator.
// It manages the photo workflow (countdown → smile → capture → display)
// and exposes a gRPC API for Buzzer and ctl to trigger the workflow.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	"github.com/chrischdi/gophotobooth/internal/galleryurl"
	pbcore "github.com/chrischdi/gophotobooth/proto/core"
	pbdslr "github.com/chrischdi/gophotobooth/proto/dslr"
	pbui "github.com/chrischdi/gophotobooth/proto/ui"
)

type coreServer struct {
	pbcore.UnimplementedCoreServer

	ui   pbui.UIClient
	dslr pbdslr.DSLRClient

	photoDir            string
	galleryBaseURL      string
	postCaptureDuration time.Duration
	autoTriggerDuration time.Duration
	sleepFn             func(time.Duration)

	mu    sync.Mutex
	timer *time.Timer
}

// Trigger starts the photo workflow if none is currently running.
func (s *coreServer) Trigger(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if !s.mu.TryLock() {
		return nil, status.Error(codes.AlreadyExists, "workflow already in progress")
	}
	go func() {
		defer s.mu.Unlock()
		s.resetTimer()
		if err := s.runWorkflow(); err != nil {
			slog.Error("workflow error", "err", err)
			_, _ = s.ui.ShowError(context.Background(), &pbui.ShowErrorRequest{Message: err.Error()})
		}
		s.resetTimer()
	}()
	return &emptypb.Empty{}, nil
}

func (s *coreServer) runWorkflow() error {
	ctx := context.Background()

	for _, count := range []int32{3, 2, 1} {
		if _, err := s.ui.ShowCountdown(ctx, &pbui.CountdownRequest{Count: count}); err != nil {
			return fmt.Errorf("ShowCountdown(%d): %w", count, err)
		}
		s.sleepFn(time.Second)
	}

	if _, err := s.ui.ShowSmile(ctx, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("ShowSmile: %w", err)
	}
	// Allow the display pipeline (Fyne render tick + panel latency) to catch
	// up before the shutter fires. DoAndWait only guarantees SetContent ran on
	// the main thread; the pixel reaching the screen takes an additional frame
	// plus any display-controller buffering (~100–150 ms on typical hardware).

	resp, err := s.dslr.TakePhoto(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("TakePhoto: %w", err)
	}

	path := filepath.Join(s.photoDir, resp.Filename)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, resp.ImageBytes, 0644); err != nil {
		return fmt.Errorf("write photo %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename photo %s: %w", path, err)
	}
	slog.Info("photo saved", "path", path)

	oldest, _ := galleryurl.OldestPhotoName(s.photoDir)
	if _, err := s.ui.ShowImage(ctx, &pbui.ShowImageRequest{
		ImageBytes: resp.ImageBytes,
		Format:     "image/jpeg",
		PhotoUrl:   galleryurl.DirectURL(s.galleryBaseURL, oldest, resp.Filename),
	}); err != nil {
		return fmt.Errorf("ShowImage: %w", err)
	}

	// Block the workflow lock for the post-capture period so no new workflow
	// can start immediately. The UI keeps showing the last photo.
	s.sleepFn(s.postCaptureDuration)

	return nil
}

func (s *coreServer) resetTimer() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.autoTriggerDuration, func() {
		slog.Info("auto-trigger fired")
		if _, err := s.Trigger(context.Background(), &emptypb.Empty{}); err != nil {
			// ALREADY_EXISTS is expected when the user just triggered manually.
			if status.Code(err) != codes.AlreadyExists {
				slog.Error("auto-trigger error", "err", err)
			}
		}
	})
}

func dialGRPC(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32*1024*1024)),
	)
}

func main() {
	var (
		listenAddr         string
		uiAddr             string
		dslrAddr           string
		photoDir           string
		galleryBaseURL     string
		postCaptureSecs    int
		autoTriggerMinutes int
	)

	cmd := &cobra.Command{
		Use:   "core",
		Short: "Photo booth orchestrator gRPC server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(photoDir, 0755); err != nil {
				return fmt.Errorf("create photo dir %s: %w", photoDir, err)
			}

			uiConn, err := dialGRPC(uiAddr)
			if err != nil {
				return fmt.Errorf("connect UI %s: %w", uiAddr, err)
			}
			defer uiConn.Close() //nolint:errcheck

			dslrConn, err := dialGRPC(dslrAddr)
			if err != nil {
				return fmt.Errorf("connect DSLR %s: %w", dslrAddr, err)
			}
			defer dslrConn.Close() //nolint:errcheck

			srv := &coreServer{
				ui:                  pbui.NewUIClient(uiConn),
				dslr:                pbdslr.NewDSLRClient(dslrConn),
				photoDir:            photoDir,
				galleryBaseURL:      galleryBaseURL,
				postCaptureDuration: time.Duration(postCaptureSecs) * time.Second,
				autoTriggerDuration: time.Duration(autoTriggerMinutes) * time.Minute,
				sleepFn:             time.Sleep,
			}

			srv.resetTimer()

			lis, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", listenAddr, err)
			}

			grpcSrv := grpc.NewServer()
			pbcore.RegisterCoreServer(grpcSrv, srv)

			slog.Info("Core gRPC server listening", "addr", listenAddr)
			return grpcSrv.Serve(lis)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", constants.CoreAddress, "gRPC listen address")
	cmd.Flags().StringVar(&uiAddr, "ui-address", constants.UIAddress, "UI service gRPC address")
	cmd.Flags().StringVar(&dslrAddr, "dslr-address", constants.DSLRAddress, "DSLR service gRPC address")
	cmd.Flags().StringVar(&photoDir, "directory", "/photos", "directory to store photos")
	cmd.Flags().StringVar(&galleryBaseURL, "gallery-base-url", "", "base URL of the gallery service (e.g. http://photobooth.local:8080); enables QR code on the UI when set")
	cmd.Flags().IntVar(&postCaptureSecs, "post-capture-seconds", 1, "seconds to block new workflows after a capture")
	cmd.Flags().IntVar(&autoTriggerMinutes, "auto-trigger-minutes", 29, "minutes of inactivity before auto-triggering a workflow")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
