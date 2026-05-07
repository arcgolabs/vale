package main

import (
	"fmt"
	"os"
)

func main() {
	if err := execute(); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "%v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}
