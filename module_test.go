package k6build

import (
	"errors"
	"testing"
)

func TestParseModule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		title         string
		module        string
		expectError   error
		expectPath    string
		expectVersion string
	}{
		{
			title:         "path with canonical version",
			module:        "github.com/path/module@v0.1.0",
			expectPath:    "github.com/path/module",
			expectVersion: "v0.1.0",
		},
		{
			title:         "path with incomplete version",
			module:        "github.com/path/module@v0.1",
			expectPath:    "github.com/path/module",
			expectVersion: "v0.1.0",
		},
		{
			title:         "path without version",
			module:        "github.com/path/module",
			expectPath:    "github.com/path/module",
			expectVersion: "latest",
		},
		{
			title:         "path with latest version",
			module:        "github.com/path/module@latest",
			expectPath:    "github.com/path/module",
			expectVersion: "latest",
		},
		{
			title:       "path with invalid version",
			module:      "github.com/path/module@1",
			expectError: ErrInvalidSemanticVersion,
		},
		{
			title:       "path with invalid incomplete version",
			module:      "github.com/path/module@v",
			expectError: ErrInvalidSemanticVersion,
		},
		{
			title:       "invalid path",
			module:      "github.com/@v1",
			expectError: ErrInvalidPath,
		},
		{
			// this is considered valid according to go's module rules
			title:       "path with only domain",
			module:      "github.com@v1",
			expectError: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			_, err := ParseModule(tc.module)
			if !errors.Is(err, tc.expectError) {
				t.Fatalf("expected %v got %v", tc.expectError, err)
			}
		})
	}
}
