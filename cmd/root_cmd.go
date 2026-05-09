package main

import (
	"fmt"

	"github.com/arcgolabs/dix"
	"github.com/spf13/cobra"
)

func init() {
	f := rootCmd.Flags()
	f.String("config", "", "path to vale HCL config")
	f.String("config-files", "", "comma-separated config files (merge order: left to right)")
	f.Bool("watch", false, "watch config and hot reload")
	f.String("log-level", "", "log level")
	f.String("raft-node-id", "", "raft node id")
	f.String("raft-bind", "", "raft bind address")
	f.String("raft-data-dir", "", "raft data directory")
	f.Bool("raft-bootstrap", false, "bootstrap raft cluster if no existing state")
	f.String("raft-initial-members", "", "comma-separated raft initial members in id=address form")
}

var rootCmd = &cobra.Command{
	Use:           "valed",
	Short:         "Vale application gateway daemon",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          runValed,
}

func runValed(cmd *cobra.Command, _ []string) error {
	rt, err := valedStandaloneApp(cmd.Flags()).Build()
	if err != nil {
		return fmt.Errorf("dix build: %w", err)
	}
	runner, err := dix.ResolveAs[*valedRunner](rt.Container())
	if err != nil {
		return fmt.Errorf("resolve runner: %w", err)
	}
	return runner.Run()
}
