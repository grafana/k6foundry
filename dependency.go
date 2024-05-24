package k6build

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

var (
	ErrInvalidDependencyFormat = errors.New("invalid dependency format")  //nolint:revive
	ErrInvalidSemanticVersion  = errors.New("invalid dependency version") //nolint:revive
	ErrInvalidPath             = errors.New("invalid dependency path")    //nolint:revive
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
func ParseModule(mod string) (Module, error) {
	path, version, found := strings.Cut(mod, "@")

	// TODO: add regexp for checking path@version
	if found && (path == "" || version == "") {
		return Module{}, fmt.Errorf("%w: %q", ErrInvalidDependencyFormat, mod)
	}

	switch version {
	case "":
		version = "latest"
	case "latest":
		break
	default:
		if !semver.IsValid(version) {
			return Module{}, fmt.Errorf("%w: %q", ErrInvalidSemanticVersion, mod)
		}
		version = semver.Canonical(version)
	}

	if err := module.CheckPath(path); err != nil {
		return Module{}, fmt.Errorf("%w: %q", ErrInvalidPath, mod)
	}

	return Module{
		PackagePath: path,
		Version:     version,
	}, nil
}
