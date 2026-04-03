// Command buzzer listens for a GPIO button press and triggers the photo workflow via Core.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/chrischdi/gophotobooth/internal/constants"
	"github.com/chrischdi/gophotobooth/pkg/buzzer"
	"github.com/chrischdi/gophotobooth/pkg/buzzer/gpio"
	pbcore "github.com/chrischdi/gophotobooth/proto/core"
)

func run(b buzzer.Buzzer, core pbcore.CoreClient) error {
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
			b, err := gpio.New(pinName)
			if err != nil {
				return fmt.Errorf("init GPIO pin %q: %w", pinName, err)
			}

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
