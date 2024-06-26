package k6foundry

import (
	"errors"
	"testing"
)

func TestParseModule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		title       string
		dependency  string
		expectError error
		expect      Module
	}{
		{
			title:      "path with canonical version",
			dependency: "github.com/path/module@v0.1.0",
			expect: Module{
				Path:    "github.com/path/module",
				Version: "v0.1.0",
			},
		},
		{
			title:      "path with incomplete version",
			dependency: "github.com/path/module@v0.1",
			expect: Module{
				Path:    "github.com/path/module",
				Version: "v0.1.0",
			},
		},
		{
			title:      "path without version",
			dependency: "github.com/path/module",
			expect: Module{
				Path:    "github.com/path/module",
				Version: "latest",
			},
		},
		{
			title:      "path with latest version",
			dependency: "github.com/path/module@latest",
			expect: Module{
				Path:    "github.com/path/module",
				Version: "latest",
			},
		},
		{
			title:       "path with invalid version",
			dependency:  "github.com/path/module@1",
			expectError: ErrInvalidDependencyFormat,
		},
		{
			title:       "path with invalid incomplete version",
			dependency:  "github.com/path/module@v",
			expectError: ErrInvalidDependencyFormat,
		},
		{
			title:       "invalid path",
			dependency:  "github.com/@v1",
			expectError: ErrInvalidDependencyFormat,
		},
		{
			// this is considered valid according to go's module rules
			title:      "path with only domain",
			dependency: "github.com@v1",
			expect: Module{
				Path:    "github.com",
				Version: "v1.0.0",
			},
		},
		{
			title:      "versioned replace",
			dependency: "github.com/path/module=github.com/another/module@v0.1.0",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "latest",
				ReplacePath:    "github.com/another/module",
				ReplaceVersion: "v0.1.0",
			},
		},
		{
			title:      "unversioned replace",
			dependency: "github.com/path/module=github.com/another/module",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "latest",
				ReplacePath:    "github.com/another/module",
				ReplaceVersion: "",
			},
		},
		{
			title:      "relative replace",
			dependency: "github.com/path/module=./another/module",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "latest",
				ReplacePath:    "./another/module",
				ReplaceVersion: "",
			},
		},
		{
			title:       "versioned relative replace",
			dependency:  "github.com/path/module=./another/module@v0.1.0",
			expectError: ErrInvalidDependencyFormat,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			module, err := ParseModule(tc.dependency)
			if !errors.Is(err, tc.expectError) {
				t.Fatalf("expected %v got %v", tc.expectError, err)
			}

			if tc.expectError == nil && tc.expect != module {
				t.Fatalf("expected %v got %v", tc.expect, module)
			}
		})
	}
}
