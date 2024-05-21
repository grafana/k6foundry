package builder

import (
	"fmt"
	"strings"
)

// Module reference a go module and its version
type Module struct {
	// The name (import path) of the Go package. If at a version > 1,
	// it should contain semantic import version (i.e. "/v2").
	// Used with `go get`.
	PackagePath string

	// The version of the Go module, as used with `go get`.
	Version string
}

// ParseModule parses a module from a string of the form path[@version]
// TODO: validate format of path and semantic version
func ParseModule(mod string) (Module, error) {
	path, version, found := strings.Cut(mod, "@")

	// TODO: add regexp for checking path@version
	if found && (path == "" || version == "") {
		return Module{}, fmt.Errorf("parsing module: invalid syntax")
	}

	if version == "" {
		version = "latest"
	}

	return Module{
		PackagePath: path,
		Version: version,
	}, nil
}
