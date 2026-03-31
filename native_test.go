package k6foundry

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/grafana/k6foundry/pkg/testutils/goproxy"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	modules := []struct {
		path    string
		version string
		source  string
	}{
		{
			path:    "go.k6.io/k6",
			version: "v0.1.0",
			source:  filepath.Join("testdata", "mods", "k6"),
		},
		{
			path:    "go.k6.io/k6",
			version: "v0.2.0",
			source:  filepath.Join("testdata", "mods", "k6"),
		},
		{
			path:    "go.k6.io/k6ext",
			version: "v0.1.0",
			source:  filepath.Join("testdata", "mods", "k6ext"),
		},
		{
			path:    "go.k6.io/k6ext",
			version: "v0.2.0",
			source:  filepath.Join("testdata", "mods", "k6ext"),
		},
		{
			path:    "go.k6.io/k6ext/v2",
			version: "v2.0.0",
			source:  filepath.Join("testdata", "mods", "k6extV2"),
		},
		{
			path:    "go.k6.io/k6ext3",
			version: "v0.1.0",
			source:  filepath.Join("testdata", "mods", "k6ext3"),
		},
		{
			path:    "private.k6.io/k6",
			version: "v0.3.0",
			source:  filepath.Join("testdata", "mods", "k6"),
		},
		{
			path:    "go.k6.io/k6/v2",
			version: "v2.0.0",
			source:  filepath.Join("testdata", "mods", "k6v2"),
		},
		{
			path:    "go.k6.io/k6/v2",
			version: "v2.1.0",
			source:  filepath.Join("testdata", "mods", "k6v2"),
		},
	}

	// creates a goproxy that serves the given modules
	proxy := goproxy.NewGoProxy()
	for _, m := range modules {
		err := proxy.AddModVersion(m.path, m.version, m.source)
		if err != nil {
			t.Fatalf("setup %v", err)
		}
	}

	goproxySrv := httptest.NewServer(proxy)

	testCases := []struct {
		title          string
		k6Version      string
		k6Repo         string
		k6MajorVersion string
		mods           []Module
		expectError    error
		expect         *BuildInfo
	}{
		{
			title:       "build k6 v0.1.0",
			k6Version:   "v0.1.0",
			mods:        []Module{},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6": "v0.1.0",
				},
			},
		},
		{
			title:       "build k6 latest",
			k6Version:   "latest",
			mods:        []Module{},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6": "v0.2.0",
				},
			},
		},
		{
			title:       "build k6 missing version (v0.3.0)",
			k6Version:   "v0.3.0",
			mods:        []Module{},
			expectError: ErrResolvingDependency,
		},
		{
			title:       "build k6 from replacement repo",
			k6Version:   "v0.3.0",
			k6Repo:      "private.k6.io/k6",
			mods:        []Module{},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6": "v0.3.0",
				},
			},
		},
		{
			title:     "build with k6ext v0.1.0",
			k6Version: "v0.1.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", Version: "v0.1.0"},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":    "v0.1.0",
					"go.k6.io/k6ext": "v0.1.0",
				},
			},
		},
		{
			title:     "build with k6ext latest",
			k6Version: "v0.1.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext"},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":    "v0.1.0",
					"go.k6.io/k6ext": "v0.2.0",
				},
			},
		},
		{
			title:     "build with missing k6ext version (v0.3.0)",
			k6Version: "v0.1.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", Version: "v0.3.0"},
			},
			expectError: ErrResolvingDependency,
		},
		{
			title:     "build versioned path k6extV2 (v2.0.0)",
			k6Version: "v0.2.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext/v2", Version: "v2.0.0"},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":       "v0.2.0",
					"go.k6.io/k6ext/v2": "v2.0.0",
				},
			},
		},
		{
			title:     "build k6ext with local module replace",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", ReplacePath: filepath.FromSlash("./testdata/mods/k6ext")},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":    "v0.1.0",
					"go.k6.io/k6ext": "v0.0.0-00010101000000-000000000000",
				},
			},
		},
		{
			title:     "build k6ext with missing replace path",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", ReplacePath: filepath.FromSlash("./testdata/mods/missing/k6ext")},
			},
			expectError: ErrResolvingDependency,
		},
		{
			title:     "build private k6ext2 module without replace path",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "private.io/k6ext2"},
			},
			expectError: ErrResolvingDependency,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":       "v0.1.0",
					"private.io/k6ext2": "v0.0.0-00010101000000-000000000000",
				},
			},
		},
		{
			title:     "build private k6ext2 module with replace path",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "private.io/k6ext2", ReplacePath: filepath.FromSlash("./testdata/mods/k6ext2")},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":       "v0.1.0",
					"private.io/k6ext2": "v0.0.0-00010101000000-000000000000",
				},
			},
		},
		{
			title:     "build private k6ext2 module v0.1.0 with replace path",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "private.io/k6ext2", Version: "v0.1.0", ReplacePath: filepath.FromSlash("./testdata/mods/k6ext2")},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6": "v0.1.0",
					// the goproxy will not serve this module
					"private.io/k6ext2": "v0.1.0",
				},
			},
		},
		{
			title:     "build mix of private and public modules",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", Version: "v0.1.0"},
				// the goproxy will not serve this module
				{Path: "private.io/k6ext2", ReplacePath: filepath.FromSlash("./testdata/mods/k6ext2")},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":       "v0.1.0",
					"go.k6.io/k6ext":    "v0.1.0",
					"private.io/k6ext2": "v0.0.0-00010101000000-000000000000",
				},
			},
		},
		{
			title:     "replace ext with ext3",
			k6Version: "v0.1.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", Version: "v0.1.0", ReplacePath: "go.k6.io/k6ext3", ReplaceVersion: "v0.1.0"},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":    "v0.1.0",
					"go.k6.io/k6ext": "v0.1.0",
				},
			},
		},
		{
			title:     "build k6 v2.0.0",
			k6Version: "v2.0.0",
			mods:      []Module{},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6/v2": "v2.0.0",
				},
			},
		},
		{
			title:          "build k6 latest v2 via K6MajorVersion",
			k6Version:      "latest",
			k6MajorVersion: "v2",
			mods:           []Module{},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6/v2": "v2.1.0",
				},
			},
		},
		{
			title:     "build k6 v2.0.0 with extension",
			k6Version: "v2.0.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", Version: "v0.1.0"},
			},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6/v2": "v2.0.0",
					"go.k6.io/k6ext": "v0.1.0",
				},
			},
		},
		{
			title:          "invalid K6MajorVersion",
			k6Version:      "latest",
			k6MajorVersion: "notvalid",
			mods:           []Module{},
			expectError:    ErrInvalidDependencyFormat,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			platform, _ := ParsePlatform("linux/amd64")

			opts := NativeFoundryOpts{
				Stdout: os.Stdout, //nolint:forbidigo
				Stderr: os.Stderr, //nolint:forbidigo
				GoOpts: GoOpts{
					CopyGoEnv: true,
					// configure go to use local goproxy to resolve go.k6.io modules
					Env: map[string]string{
						"GOPROXY":   goproxySrv.URL,
						"GONOPROXY": "none",
						"GOPRIVATE": "go.k6.io,private.k6.io",
						"GONOSUMDB": "go.k6.io,private.k6.io",
					},
					TmpCache: true,
				},
				K6Repo:         tc.k6Repo,
				K6MajorVersion: tc.k6MajorVersion,
			}

			b, err := NewNativeFoundry(context.Background(), opts)
			if err != nil {
				t.Fatalf("setting up test %v", err)
			}

			outFile := &bytes.Buffer{}
			buildInfo, err := b.Build(
				context.Background(),
				platform,
				tc.k6Version,
				tc.mods,
				[]Module{},
				[]string{},
				outFile,
			)

			if !errors.Is(err, tc.expectError) {
				t.Fatalf("expected %v got %v", tc.expectError, err)
			}

			if tc.expectError != nil {
				return
			}

			if outFile.Len() == 0 {
				t.Fatal("out file is empty")
			}

			if !reflect.DeepEqual(buildInfo, tc.expect) {
				t.Fatalf("expected %v got %v", tc.expect, buildInfo)
			}
		})
	}
}

// TestBuildVersionedExtensions tests the behaviour when extensions have explicit k6 version
// dependencies. Both k6 v1 and k6 v2 are available in the proxy, mirroring a real-world
// public module registry where all major versions coexist.
func TestBuildVersionedExtensions(t *testing.T) {
	t.Parallel()

	proxy := goproxy.NewGoProxy()
	for _, m := range []struct {
		path, version, source string
	}{
		{"go.k6.io/k6", "v0.1.0", filepath.Join("testdata", "mods", "k6")},
		{"go.k6.io/k6/v2", "v2.0.0", filepath.Join("testdata", "mods", "k6v2")},
		{"go.k6.io/k6extforv1", "v0.1.0", filepath.Join("testdata", "mods", "k6extforv1")},
		{"go.k6.io/k6extforv2", "v0.1.0", filepath.Join("testdata", "mods", "k6extforv2")},
	} {
		if err := proxy.AddModVersion(m.path, m.version, m.source); err != nil {
			t.Fatalf("setup proxy: %v", err)
		}
	}
	proxySrv := httptest.NewServer(proxy)

	testCases := []struct {
		title              string
		k6Version          string
		mods               []Module
		expect             *BuildInfo
		expectWarningCodes []WarningCode
	}{
		{
			// Compatible: extension requires go.k6.io/k6, building k6 v1. No conflict.
			title:     "k6 v1 + v1 extension succeeds without warnings",
			k6Version: "v0.1.0",
			mods:      []Module{{Path: "go.k6.io/k6extforv1", Version: "v0.1.0"}},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":         "v0.1.0",
					"go.k6.io/k6extforv1": "v0.1.0",
				},
			},
		},
		{
			// Compatible: extension requires go.k6.io/k6/v2, building k6 v2. No conflict.
			title:     "k6 v2 + v2 extension succeeds without warnings",
			k6Version: "v2.0.0",
			mods:      []Module{{Path: "go.k6.io/k6extforv2", Version: "v0.1.0"}},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6/v2":      "v2.0.0",
					"go.k6.io/k6extforv2": "v0.1.0",
				},
			},
		},
		{
			// Incompatible: extension requires go.k6.io/k6 (v1) but building k6 v2.
			// Go resolves the extension's transitive go.k6.io/k6 dep and compiles both k6
			// versions into the binary. The v1 extension is dead code — it registers with
			// the k6 v1 runtime, which is never called by k6 v2. A WarnK6VersionConflict
			// warning is emitted so callers can detect and act on this.
			title:     "k6 v2 + v1 extension succeeds with WarnK6VersionConflict",
			k6Version: "v2.0.0",
			mods:      []Module{{Path: "go.k6.io/k6extforv1", Version: "v0.1.0"}},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6/v2":      "v2.0.0",
					"go.k6.io/k6extforv1": "v0.1.0",
				},
			},
			expectWarningCodes: []WarningCode{WarnK6VersionConflict},
		},
		{
			// Symmetric incompatible case: extension requires go.k6.io/k6/v2 but building k6 v1.
			// The v2 extension registers with the k6 v2 runtime, which is never called by k6 v1.
			title:     "k6 v1 + v2 extension succeeds with WarnK6VersionConflict",
			k6Version: "v0.1.0",
			mods:      []Module{{Path: "go.k6.io/k6extforv2", Version: "v0.1.0"}},
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":         "v0.1.0",
					"go.k6.io/k6extforv2": "v0.1.0",
				},
			},
			expectWarningCodes: []WarningCode{WarnK6VersionConflict},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			platform, _ := ParsePlatform("linux/amd64")

			opts := NativeFoundryOpts{
				Stdout: os.Stdout, //nolint:forbidigo
				Stderr: os.Stderr, //nolint:forbidigo
				GoOpts: GoOpts{
					CopyGoEnv: true,
					Env: map[string]string{
						"GOPROXY":   proxySrv.URL,
						"GONOPROXY": "none",
						"GOPRIVATE": "go.k6.io",
						"GONOSUMDB": "go.k6.io",
					},
					TmpCache: true,
				},
			}

			b, err := NewNativeFoundry(context.Background(), opts)
			if err != nil {
				t.Fatalf("setting up test %v", err)
			}

			buildInfo, err := b.Build(
				context.Background(),
				platform,
				tc.k6Version,
				tc.mods,
				[]Module{},
				[]string{},
				&bytes.Buffer{},
			)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			// Compare BuildInfo without Warnings: message strings are checked separately below.
			got := *buildInfo
			got.Warnings = nil
			if !reflect.DeepEqual(&got, tc.expect) {
				t.Fatalf("expected %v got %v", tc.expect, buildInfo)
			}

			var gotCodes []WarningCode
			for _, w := range buildInfo.Warnings {
				gotCodes = append(gotCodes, w.Code)
			}
			if !reflect.DeepEqual(gotCodes, tc.expectWarningCodes) {
				t.Fatalf("expected warning codes %v got %v", tc.expectWarningCodes, gotCodes)
			}
		})
	}
}
