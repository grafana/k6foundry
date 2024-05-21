// package main implements the CLI root command for the k6build tool
package main

import (
	"fmt"
	"os"
)

//nolint:all
func main() {
	root := newRootCmd()

	err := root.Execute()
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}
}
