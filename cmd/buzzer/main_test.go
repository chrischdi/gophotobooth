package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/testutil"
	pbcore "github.com/chrischdi/gophotobooth/proto/core"
)

// mockBuzzer simulates exactly N button presses, then returns an error.
type mockBuzzer struct {
	mu      sync.Mutex
	presses int
	called  int
}

func (m *mockBuzzer) Wait() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.called >= m.presses {
		return errors.New("mock: no more presses")
	}
	m.called++
	return nil
}

// recordingCoreServer counts Trigger calls and returns a configured error.
type recordingCoreServer struct {
	pbcore.UnimplementedCoreServer
	mu       sync.Mutex
	triggers int
	err      error
}

func (s *recordingCoreServer) Trigger(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggers++
	return &emptypb.Empty{}, s.err
}

func (s *recordingCoreServer) getTriggers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.triggers
}

// newCoreClient builds a CoreClient backed by an in-process recordingCoreServer.
func newCoreClient(t *testing.T, srv *recordingCoreServer) pbcore.CoreClient {
	t.Helper()
	conn := testutil.NewBufconnConn(t, func(s *grpc.Server) {
		pbcore.RegisterCoreServer(s, srv)
	})
	return pbcore.NewCoreClient(conn)
}

func TestRun_SinglePress_TriggersCoreOnce(t *testing.T) {
	coreSrv := &recordingCoreServer{}
	b := &mockBuzzer{presses: 1}
	err := run(b, newCoreClient(t, coreSrv))
	require.Error(t, err, "run should return the mock buzzer exhaustion error")
	assert.Equal(t, 1, coreSrv.getTriggers())
}

func TestRun_MultiplePresses_TriggersMultipleTimes(t *testing.T) {
	coreSrv := &recordingCoreServer{}
	b := &mockBuzzer{presses: 3}
	err := run(b, newCoreClient(t, coreSrv))
	require.Error(t, err)
	assert.Equal(t, 3, coreSrv.getTriggers())
}

func TestRun_CoreAlreadyExists_Continues(t *testing.T) {
	coreSrv := &recordingCoreServer{err: status.Error(codes.AlreadyExists, "workflow running")}
	b := &mockBuzzer{presses: 2}
	err := run(b, newCoreClient(t, coreSrv))
	require.Error(t, err)
	// Loop must not stop on AlreadyExists — both presses are attempted
	assert.Equal(t, 2, coreSrv.getTriggers())
}

func TestRun_CoreInternalError_Continues(t *testing.T) {
	coreSrv := &recordingCoreServer{err: status.Error(codes.Internal, "internal error")}
	b := &mockBuzzer{presses: 2}
	err := run(b, newCoreClient(t, coreSrv))
	require.Error(t, err)
	assert.Equal(t, 2, coreSrv.getTriggers())
}

func TestRun_BuzzerErrorStops(t *testing.T) {
	coreSrv := &recordingCoreServer{}
	b := &mockBuzzer{presses: 0} // immediate error
	err := run(b, newCoreClient(t, coreSrv))
	require.Error(t, err)
	assert.Equal(t, 0, coreSrv.getTriggers())
}
