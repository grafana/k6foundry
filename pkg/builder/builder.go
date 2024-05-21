// package builder contains k6 builder logic.
package builder

import (
	"context"
	"io"
)

// Builder defines the interface for building a k6 binary
type Builder interface {
	// Build returns a custom k6 binary for the given version including a set of dependencies
	Build(ctx context.Context, platform Platform, k6Version string, mods []Module, out io.Writer) error
}

// NewDefaultBuilder return a new default Builder
func NewDefaultBuilder(ctx context.Context) (Builder, error) {
	return newNativeBuilder(ctx)
}
