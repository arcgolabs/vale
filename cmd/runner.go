package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/vale"
	"github.com/samber/oops"
)

const defaultShutdownTimeout = 10 * time.Second

type valedRunner struct {
	gateway         *vale.Gateway
	logger          *slog.Logger
	bus             eventx.BusRuntime
	shutdownTimeout time.Duration
	signals         chan os.Signal
}

func provideRunner(gateway *vale.Gateway, logger *slog.Logger, bus eventx.BusRuntime) *valedRunner {
	return &valedRunner{
		gateway:         gateway,
		logger:          logger,
		bus:             bus,
		shutdownTimeout: defaultShutdownTimeout,
		signals:         make(chan os.Signal, 1),
	}
}

func (r *valedRunner) Run() error {
	if err := r.gateway.Start(context.Background()); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "start valed")
	}
	r.logger.Info("valed started")

	signal.Notify(r.signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-r.signals
	signal.Stop(r.signals)
	r.logger.Info("shutdown signal received", "signal", sig.String())

	r.stopGateway()
	r.closeEventBus()
	r.logger.Info("valed stopped")
	if err := logx.Close(r.logger); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "close logger")
	}
	return nil
}

func (r *valedRunner) stopGateway() {
	ctx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()
	if err := r.gateway.Stop(ctx); err != nil {
		r.logger.Error("gateway stop failed", "error", err)
	}
}

func (r *valedRunner) closeEventBus() {
	if r.bus == nil {
		return
	}
	if err := r.bus.Close(); err != nil {
		r.logger.Error("event bus close failed", "error", err)
	}
}
