// Package k6build contains k6 build logic.
package k6build

import (
	"context"
	"io"
)

// Builder defines the interface for building a k6 binary
type Builder interface {
	// Build returns a custom k6 binary for the given version including a set of dependencies
	Build(
		ctx context.Context,
		platform Platform,
		k6Version string,
		mods []Module,
		buildOpts []string,
		out io.Writer,
	) error
}
