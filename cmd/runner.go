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

type veladRunner struct {
	gateway         *vela.Gateway
	logger          *slog.Logger
	bus             eventx.BusRuntime
	shutdownTimeout time.Duration
	signals         chan os.Signal
}

func provideRunner(gateway *vela.Gateway, logger *slog.Logger, bus eventx.BusRuntime) *veladRunner {
	return &veladRunner{
		gateway:         gateway,
		logger:          logger,
		bus:             bus,
		shutdownTimeout: defaultShutdownTimeout,
		signals:         make(chan os.Signal, 1),
	}
}

func (r *veladRunner) Run() error {
	if err := r.gateway.Start(context.Background()); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "start velad")
	}
	r.logger.Info("velad started")

	signal.Notify(r.signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-r.signals
	signal.Stop(r.signals)
	r.logger.Info("shutdown signal received", "signal", sig.String())

	r.stopGateway()
	r.closeEventBus()
	r.logger.Info("velad stopped")
	if err := logx.Close(r.logger); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "close logger")
	}
	return nil
}

func (r *veladRunner) stopGateway() {
	ctx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()
	if err := r.gateway.Stop(ctx); err != nil {
		r.logger.Error("gateway stop failed", "error", err)
	}
}

func (r *veladRunner) closeEventBus() {
	if r.bus == nil {
		return
	}
	if err := r.bus.Close(); err != nil {
		r.logger.Error("event bus close failed", "error", err)
	}
}
