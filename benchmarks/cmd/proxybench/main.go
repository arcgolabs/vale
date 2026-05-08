// Command proxybench runs a lightweight HTTP reverse-proxy benchmark.
package main

import (
	"fmt"
	"os"
)

const toolName = "vale-proxybench"

func main() {
	cfg, err := parseConfig()
	if err != nil {
		exitWithError(2, err)
	}
	if err := run(cfg); err != nil {
		exitWithError(1, err)
	}
}

func exitWithError(code int, err error) {
	if _, writeErr := fmt.Fprintf(os.Stderr, "proxybench: %v\n", err); writeErr != nil {
		os.Exit(1)
	}
	os.Exit(code)
}
