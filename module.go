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

	if err := module.CheckPath(path); err != nil {
		return Module{}, fmt.Errorf("%w: %q", ErrInvalidPath, mod)
	}

	return Module{
		PackagePath: path,
		Version:     version,
	}, nil
}

// VersionedPath returns the Module's PackagePath with the major component of moduleVersion added,
// if it is a valid semantic version and is > 1
// Examples:
// - PackagePath="foo" and Version="v1.0.0" returns "foo"
// - PackagePath="foo" and Version="v2.0.0" returns "foo/v2"
// - PackagePath="foo/v2" and vVersion="v3.0.0" returns an error
// - PackagePath="foo" and Version="latest" returns "foo"
func (mod Module) VersionedPath() (string, error) {
	// if not is a semantic version return (could have been a commit SHA or 'latest')
	if !semver.IsValid(mod.Version) {
		return mod.PackagePath, nil
	}
	major := semver.Major(mod.Version)

	// if the module path has a major version at the end, check for inconsistencies
	if moduleVersionRegexp.MatchString(mod.PackagePath) {
		modPathVer := filepath.Base(mod.PackagePath)
		if modPathVer != major {
			return "", fmt.Errorf("invalid version for versioned package %q: %q", mod.PackagePath, mod.Version)
		}
		return mod.PackagePath, nil
	}

	// if module path does not specify major version, add it if > 1
	switch major {
	case "v0", "v1":
		return mod.PackagePath, nil
	default:
		return filepath.Join(mod.PackagePath, major), nil
	}
}
