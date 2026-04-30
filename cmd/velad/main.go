package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/gateway/app/standalone"
)

func main() {
	bootstrap, err := standalone.LoadBootstrap()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load bootstrap config: %v\n", err)
		os.Exit(1)
	}
	flag.StringVar(&bootstrap.ConfigPath, "config", bootstrap.ConfigPath, "path to vela hcl config")
	flag.StringVar(&bootstrap.ConfigFiles, "config-files", bootstrap.ConfigFiles, "comma-separated config files (merge order: left to right)")
	flag.BoolVar(&bootstrap.Watch, "watch", bootstrap.Watch, "watch config file and hot reload")
	flag.StringVar(&bootstrap.LogLevel, "log-level", bootstrap.LogLevel, "log level")
	flag.BoolVar(&bootstrap.RaftEnabled, "raft-enabled", bootstrap.RaftEnabled, "enable raft control-plane node")
	flag.StringVar(&bootstrap.RaftNodeID, "raft-node-id", bootstrap.RaftNodeID, "raft node id")
	flag.StringVar(&bootstrap.RaftBind, "raft-bind", bootstrap.RaftBind, "raft bind address")
	flag.StringVar(&bootstrap.RaftDataDir, "raft-data-dir", bootstrap.RaftDataDir, "raft data directory")
	flag.BoolVar(&bootstrap.RaftBoot, "raft-bootstrap", bootstrap.RaftBoot, "bootstrap raft cluster if no existing state")
	flag.Parse()

	app := standalone.NewApp(bootstrap)
	containerRuntime, err := app.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build app container: %v\n", err)
		os.Exit(1)
	}
	runner, err := dix.ResolveAs[*standalone.Runner](containerRuntime.Container())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve runner: %v\n", err)
		os.Exit(1)
	}
	if err := runner.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start velad: %v\n", err)
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = runner.Stop(ctx)
}
