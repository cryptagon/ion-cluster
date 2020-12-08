// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"fmt"
	"os"

	"github.com/pion/ion-cluster/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
