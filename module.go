package k6build

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

var (
	moduleVersionRegexp = regexp.MustCompile(`.+/v(\d+)$`)

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

	err := module.CheckPath(path)
	if err != nil {
		return Module{}, fmt.Errorf("%w: %q", ErrInvalidPath, mod)
	}

	// TODO: should we enforce the versioned path or reject if it not conformant?
	path, err = versionedPath(path, version)
	if err != nil {
		return Module{}, fmt.Errorf("%w: %q", ErrInvalidPath, mod)
	}

	return Module{
		PackagePath: path,
		Version:     version,
	}, nil
}

// VersionedPath returns a module path with the major component of version added,
// if it is a valid semantic version and is > 1
// Examples:
// - PackagePath="foo" and Version="v1.0.0" returns "foo"
// - PackagePath="foo" and Version="v2.0.0" returns "foo/v2"
// - PackagePath="foo/v2" and vVersion="v3.0.0" returns an error
// - PackagePath="foo" and Version="latest" returns "foo"
func versionedPath(path string, version string) (string, error) {
	// if not is a semantic version return (could have been a commit SHA or 'latest')
	if !semver.IsValid(version) {
		return path, nil
	}
	major := semver.Major(version)

	// if the module path has a major version at the end, check for inconsistencies
	if moduleVersionRegexp.MatchString(path) {
		modPathVer := filepath.Base(path)
		if modPathVer != major {
			return "", fmt.Errorf("invalid version for versioned package %q: %q", path, version)
		}
		return path, nil
	}

	// if module path does not specify major version, add it if > 1
	switch major {
	case "v0", "v1":
		return path, nil
	default:
		return filepath.Join(path, major), nil
	}
}
