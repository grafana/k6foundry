package k6foundry

import (
	"errors"
	"testing"
)

func TestK6ModulePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		title         string
		version       string
		majorOverride string
		expect        string
		expectError   error
	}{
		{
			title:   "v0 version",
			version: "v0.55.0",
			expect:  "go.k6.io/k6",
		},
		{
			title:   "v1 version",
			version: "v1.0.0",
			expect:  "go.k6.io/k6",
		},
		{
			title:   "v2 version",
			version: "v2.0.0",
			expect:  "go.k6.io/k6/v2",
		},
		{
			title:   "v3 version",
			version: "v3.0.0",
			expect:  "go.k6.io/k6/v3",
		},
		{
			title:   "pre-release v2 version",
			version: "v2.0.0-rc1",
			expect:  "go.k6.io/k6/v2",
		},
		{
			title:   "latest without override",
			version: "latest",
			expect:  "go.k6.io/k6",
		},
		{
			title:         "latest with v2 override",
			version:       "latest",
			majorOverride: "v2",
			expect:        "go.k6.io/k6/v2",
		},
		{
			title:         "latest with v3 override",
			version:       "latest",
			majorOverride: "v3",
			expect:        "go.k6.io/k6/v3",
		},
		{
			title:         "latest with v1 override",
			version:       "latest",
			majorOverride: "v1",
			expect:        "go.k6.io/k6",
		},
		{
			title:         "latest with v0 override",
			version:       "latest",
			majorOverride: "v0",
			expect:        "go.k6.io/k6",
		},
		{
			title:         "commit SHA with v2 override",
			version:       "abc123def456",
			majorOverride: "v2",
			expect:        "go.k6.io/k6/v2",
		},
		{
			title:         "semver takes precedence over override",
			version:       "v2.0.0",
			majorOverride: "v3",
			expect:        "go.k6.io/k6/v2",
		},
		{
			title:         "invalid majorOverride",
			version:       "latest",
			majorOverride: "notvalid",
			expectError:   ErrInvalidDependencyFormat,
		},
		{
			title:         "majorOverride without v prefix",
			version:       "latest",
			majorOverride: "2",
			expectError:   ErrInvalidDependencyFormat,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			result, err := k6ModulePath(tc.version, tc.majorOverride)
			if !errors.Is(err, tc.expectError) {
				t.Fatalf("expected error %v got %v", tc.expectError, err)
			}

			if tc.expectError == nil && result != tc.expect {
				t.Fatalf("expected %q got %q", tc.expect, result)
			}
		})
	}
}

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
			title:      "path without version",
			dependency: "github.com/path/module",
			expect: Module{
				Path:    "github.com/path/module",
				Version: "",
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
			title:       "invalid path",
			dependency:  "github.com/@v1",
			expectError: ErrInvalidDependencyFormat,
		},
		{
			title:      "versioned replace",
			dependency: "github.com/path/module=github.com/another/module@v0.1.0",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "",
				ReplacePath:    "github.com/another/module",
				ReplaceVersion: "v0.1.0",
			},
		},
		{
			title:      "unversioned replace",
			dependency: "github.com/path/module=github.com/another/module",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "",
				ReplacePath:    "github.com/another/module",
				ReplaceVersion: "",
			},
		},
		{
			title:      "relative replace",
			dependency: "github.com/path/module=./another/module",
			expect: Module{
				Path:           "github.com/path/module",
				Version:        "",
				ReplacePath:    "./another/module",
				ReplaceVersion: "",
			},
		},
		{
			title:       "versioned relative replace",
			dependency:  "github.com/path/module=./another/module@v0.1.0",
			expectError: ErrInvalidDependencyFormat,
		},
		{
			title:      "only module name",
			dependency: "module",
			expect: Module{
				Path: "module",
			},
		},
		{
			title:       "empty module",
			dependency:  "",
			expectError: ErrInvalidDependencyFormat,
		},
	}

	for _, tc := range testCases {
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
