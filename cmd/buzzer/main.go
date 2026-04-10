// Command buzzer listens for a GPIO button press and triggers the photo workflow via Core.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/warthog618/go-gpiocdev"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	pbcore "github.com/chrischdi/gophotobooth/proto/core"
)

const (
	gpiochip    = "gpiochip0"
	debounceDur = 300 * time.Millisecond
)

type waiter interface {
	Wait() error
}

type gpioButton struct {
	line        *gpiocdev.Line
	ch          chan struct{}
	lastTrigger time.Time
}

func newGPIOButton(pinName string) (*gpioButton, error) {
	var offset int
	if _, err := fmt.Sscanf(pinName, "GPIO%d", &offset); err != nil {
		return nil, fmt.Errorf("invalid pin name %q (expected e.g. GPIO17)", pinName)
	}

	b := &gpioButton{ch: make(chan struct{}, 1)}

	line, err := gpiocdev.RequestLine(gpiochip, offset,
		gpiocdev.WithBiasDisabled,
		gpiocdev.WithFallingEdge,
		gpiocdev.WithEventHandler(func(_ gpiocdev.LineEvent) {
			if time.Since(b.lastTrigger) < debounceDur {
				return
			}
			b.lastTrigger = time.Now()
			select {
			case b.ch <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("request line %d on %s: %w", offset, gpiochip, err)
	}

	b.line = line
	return b, nil
}

func (b *gpioButton) Wait() error {
	<-b.ch
	return nil
}

func run(b waiter, core pbcore.CoreClient) error {
	for {
		if err := b.Wait(); err != nil {
			return fmt.Errorf("buzzer wait: %w", err)
		}
		slog.Info("button pressed — triggering workflow")
		_, err := core.Trigger(context.Background(), &emptypb.Empty{})
		if err != nil {
			// Log but don't stop — workflow may already be running.
			slog.Warn("trigger error", "err", err)
		}
	}
}

func main() {
	var (
		coreAddr string
		pinName  string
	)

	cmd := &cobra.Command{
		Use:   "buzzer",
		Short: "GPIO button listener that triggers the photo workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := newGPIOButton(pinName)
			if err != nil {
				return fmt.Errorf("init GPIO pin %q: %w", pinName, err)
			}
			defer b.line.Close() //nolint:errcheck

			conn, err := grpc.NewClient(coreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return fmt.Errorf("connect Core %s: %w", coreAddr, err)
			}
			defer conn.Close() //nolint:errcheck

			slog.Info("buzzer running", "pin", pinName, "core", coreAddr)
			return run(b, pbcore.NewCoreClient(conn))
		},
	}

	cmd.Flags().StringVar(&coreAddr, "core-address", constants.CoreAddress, "Core service gRPC address")
	cmd.Flags().StringVar(&pinName, "pin", "GPIO17", "GPIO pin name for the button")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
