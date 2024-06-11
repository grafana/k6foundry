// package main implements the CLI root command for the k6build tool
package main

import (
	"fmt"
	"os"

	"github.com/grafana/k6build"
)

//nolint:all
func main() {
	root := k6build.NewCmd()

	err := root.Execute()
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}
}
