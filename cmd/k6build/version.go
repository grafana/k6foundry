package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// build version, can be set at build time using ldflags:
// -ldflags='-X main.version=<version>'
var version = "dev"

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "version",
		Long:  "returns the current version of the k6build tool",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("k6build %s\n", version)
		},
	}

	return cmd
}
