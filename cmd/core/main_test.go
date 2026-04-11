package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/testutil"
	pbdslr "github.com/chrischdi/gophotobooth/proto/dslr"
	pbui "github.com/chrischdi/gophotobooth/proto/ui"
)

// recordingUIServer records the sequence of RPC calls made to it.
type recordingUIServer struct {
	pbui.UnimplementedUIServer
	mu          sync.Mutex
	calls       []string
	imageReqs   []*pbui.ShowImageRequest
	countdownErr int32 // if non-zero, return error on ShowCountdown for this count value
}

func (s *recordingUIServer) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *recordingUIServer) ShowCountdown(_ context.Context, req *pbui.CountdownRequest) (*emptypb.Empty, error) {
	s.record(fmt.Sprintf("ShowCountdown(%d)", req.Count))
	if s.countdownErr != 0 && req.Count == s.countdownErr {
		return nil, status.Errorf(codes.Internal, "injected countdown error for %d", req.Count)
	}
	return &emptypb.Empty{}, nil
}

func (s *recordingUIServer) ShowSmile(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.record("ShowSmile")
	return &emptypb.Empty{}, nil
}

func (s *recordingUIServer) ShowImage(_ context.Context, req *pbui.ShowImageRequest) (*emptypb.Empty, error) {
	s.record("ShowImage")
	s.mu.Lock()
	s.imageReqs = append(s.imageReqs, req)
	s.mu.Unlock()
	return &emptypb.Empty{}, nil
}

func (s *recordingUIServer) ShowError(_ context.Context, _ *pbui.ShowErrorRequest) (*emptypb.Empty, error) {
	s.record("ShowError")
	return &emptypb.Empty{}, nil
}

func (s *recordingUIServer) getCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

// fixedDSLRServer returns a canned photo or a configured error.
type fixedDSLRServer struct {
	pbdslr.UnimplementedDSLRServer
	imageBytes []byte
	filename   string
	err        error
	delay      time.Duration
}

func (s *fixedDSLRServer) TakePhoto(_ context.Context, _ *emptypb.Empty) (*pbdslr.TakePhotoResponse, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.err != nil {
		return nil, s.err
	}
	return &pbdslr.TakePhotoResponse{
		ImageBytes: s.imageBytes,
		Filename:   s.filename,
	}, nil
}

// buildServer wires up a coreServer backed by in-process gRPC stub servers.
func buildServer(t *testing.T, uiSrv pbui.UIServer, dslrSrv pbdslr.DSLRServer, photoDir string) *coreServer {
	t.Helper()

	uiConn := testutil.NewBufconnConn(t, func(s *grpc.Server) {
		pbui.RegisterUIServer(s, uiSrv)
	})
	dslrConn := testutil.NewBufconnConn(t, func(s *grpc.Server) {
		pbdslr.RegisterDSLRServer(s, dslrSrv)
	})

	return &coreServer{
		ui:                  pbui.NewUIClient(uiConn),
		dslr:                pbdslr.NewDSLRClient(dslrConn),
		photoDir:            photoDir,
		postCaptureDuration: 0,
		autoTriggerDuration: 999 * time.Minute,
		sleepFn:             func(time.Duration) {}, // no-op for fast tests
	}
}

// smallJPEG is a minimal valid JPEG (1×1 pixel).
var smallJPEG = []byte{
	0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xdb, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0a, 0x0c, 0x14, 0x0d, 0x0c, 0x0b, 0x0b, 0x0c, 0x19, 0x12,
	0x13, 0x0f, 0x14, 0x1d, 0x1a, 0x1f, 0x1e, 0x1d, 0x1a, 0x1c, 0x1c, 0x20,
	0x24, 0x2e, 0x27, 0x20, 0x22, 0x2c, 0x23, 0x1c, 0x1c, 0x28, 0x37, 0x29,
	0x2c, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1f, 0x27, 0x39, 0x3d, 0x38, 0x32,
	0x3c, 0x2e, 0x33, 0x34, 0x32, 0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xff, 0xc4, 0x00, 0x1f, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0xff, 0xc4, 0x00, 0xb5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7d,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xa1, 0x08,
	0x23, 0x42, 0xb1, 0xc1, 0x15, 0x52, 0xd1, 0xf0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0a, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2a, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4a, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7a, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8a, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3,
	0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6,
	0xb7, 0xb8, 0xb9, 0xba, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9,
	0xca, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xe1, 0xe2,
	0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xf1, 0xf2, 0xf3, 0xf4,
	0xf5, 0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01,
	0x00, 0x00, 0x3f, 0x00, 0xfb, 0xd7, 0xff, 0xd9,
}

func TestRunWorkflow_HappyPath(t *testing.T) {
	uiSrv := &recordingUIServer{}
	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "test.jpg"}
	photoDir := t.TempDir()

	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	err := srv.runWorkflow()
	require.NoError(t, err)

	want := []string{"ShowCountdown(3)", "ShowCountdown(2)", "ShowCountdown(1)", "ShowSmile", "ShowImage"}
	assert.Equal(t, want, uiSrv.getCalls())

	// Photo written to disk
	data, err := os.ReadFile(photoDir + "/test.jpg")
	require.NoError(t, err)
	assert.Equal(t, smallJPEG, data)

	// ShowImage had correct format
	require.Len(t, uiSrv.imageReqs, 1)
	assert.Equal(t, "image/jpeg", uiSrv.imageReqs[0].Format)
	assert.Equal(t, smallJPEG, uiSrv.imageReqs[0].ImageBytes)
}

func TestRunWorkflow_PhotoURL(t *testing.T) {
	uiSrv := &recordingUIServer{}
	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "2026-03-22T12-00-00.jpg"}
	photoDir := t.TempDir()

	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	srv.galleryBaseURL = "http://photobooth.local:8080"

	require.NoError(t, srv.runWorkflow())

	require.Len(t, uiSrv.imageReqs, 1)
	photoURL := uiSrv.imageReqs[0].PhotoUrl
	assert.Contains(t, photoURL, "/g/")
	assert.NotContains(t, photoURL, "#")
}

func TestRunWorkflow_DSLRError(t *testing.T) {
	uiSrv := &recordingUIServer{}
	dslrSrv := &fixedDSLRServer{err: status.Error(codes.Internal, "camera jam")}
	photoDir := t.TempDir()

	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	err := srv.runWorkflow()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TakePhoto")

	calls := uiSrv.getCalls()
	assert.NotContains(t, calls, "ShowImage")

	entries, _ := os.ReadDir(photoDir)
	assert.Empty(t, entries, "no photo should be written on DSLR error")
}

func TestRunWorkflow_UICountdownError(t *testing.T) {
	uiSrv := &recordingUIServer{countdownErr: 2}
	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "test.jpg"}
	photoDir := t.TempDir()

	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	err := srv.runWorkflow()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ShowCountdown(2)")

	calls := uiSrv.getCalls()
	assert.NotContains(t, calls, "ShowSmile")
	assert.NotContains(t, calls, "ShowImage")

	entries, _ := os.ReadDir(photoDir)
	assert.Empty(t, entries)
}

func TestRunWorkflow_WriteFileError(t *testing.T) {
	uiSrv := &recordingUIServer{}
	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "test.jpg"}

	srv := buildServer(t, uiSrv, dslrSrv, "/nonexistent/path/that/cannot/be/created")
	err := srv.runWorkflow()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write photo")
	assert.NotContains(t, uiSrv.getCalls(), "ShowImage")
}

func TestTrigger_ConcurrentRejectsSecond(t *testing.T) {
	uiSrv := &recordingUIServer{}
	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "test.jpg", delay: 100 * time.Millisecond}
	photoDir := t.TempDir()

	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	srv.autoTriggerDuration = 999 * time.Minute

	// First trigger — starts workflow in goroutine
	_, err := srv.Trigger(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)

	// Second trigger immediately — should be rejected
	_, err = srv.Trigger(context.Background(), &emptypb.Empty{})
	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, status.Code(err))

	// Wait for the workflow to complete (mutex released)
	require.Eventually(t, func() bool {
		_, err := srv.Trigger(context.Background(), &emptypb.Empty{})
		if err == nil {
			return true
		}
		return status.Code(err) != codes.AlreadyExists
	}, 5*time.Second, 50*time.Millisecond)
}

func TestTrigger_AllowsAfterCompletion(t *testing.T) {
	uiSrv := &recordingUIServer{}
	photoDir := t.TempDir()

	dslrSrv := &fixedDSLRServer{imageBytes: smallJPEG, filename: "a.jpg"}
	srv := buildServer(t, uiSrv, dslrSrv, photoDir)
	srv.autoTriggerDuration = 999 * time.Minute

	// First trigger
	_, err := srv.Trigger(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)

	// Wait for mutex to be released
	require.Eventually(t, func() bool {
		return srv.mu.TryLock()
	}, 5*time.Second, 10*time.Millisecond)
	srv.mu.Unlock()

	// Second trigger should succeed
	_, err = srv.Trigger(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
}
