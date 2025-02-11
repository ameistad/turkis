package main

import (
	"fmt"
	"os"

	"github.com/ameistad/turkis/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Print error once, then exit
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
