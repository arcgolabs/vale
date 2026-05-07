package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/vela"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	f := rootCmd.Flags()
	f.String("config", "", "path to vela HCL config")
	f.String("config-files", "", "comma-separated config files (merge order: left to right)")
	f.Bool("watch", false, "watch config and hot reload")
	f.String("log-level", "", "log level")
	f.Bool("raft-enabled", false, "enable raft control-plane node")
	f.String("raft-node-id", "", "raft node id")
	f.String("raft-bind", "", "raft bind address")
	f.String("raft-data-dir", "", "raft data directory")
	f.Bool("raft-bootstrap", false, "bootstrap raft cluster if no existing state")
}

var rootCmd = &cobra.Command{
	Use:           "velad",
	Short:         "Vela application gateway daemon",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          runVelad,
}

func runVelad(cmd *cobra.Command, _ []string) error {
	rt, err := veladStandaloneApp(cmd.Flags()).Build()
	if err != nil {
		return fmt.Errorf("dix build: %w", err)
	}
	c := rt.Container()

	gateway, err := dix.ResolveAs[*vela.Gateway](c)
	if err != nil {
		return fmt.Errorf("resolve gateway: %w", err)
	}
	logger, err := dix.ResolveAs[*slog.Logger](c)
	if err != nil {
		return fmt.Errorf("resolve logger: %w", err)
	}
	bus, err := dix.ResolveAs[eventx.BusRuntime](c)
	if err != nil {
		return fmt.Errorf("resolve event bus: %w", err)
	}

	if err := gateway.Start(context.Background()); err != nil {
		return fmt.Errorf("start velad: %w", err)
	}
	logger.Info("velad started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	logger.Info("shutdown signal received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := gateway.Stop(ctx); err != nil {
		logger.Error("gateway stop failed", "error", err)
	}
	if bus != nil {
		if err := bus.Close(); err != nil {
			logger.Error("event bus close failed", "error", err)
		}
	}
	logger.Info("velad stopped")
	if err := logx.Close(logger); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "close logger")
	}
	return nil
}
