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
			path:    "go.k6.io/k6ext/v2",
			version: "v2.0.0",
			source:  filepath.Join("testdata", "mods", "k6extV2"),
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
		title       string
		k6Version   string
		mods        []Module
		expectError error
		expect      *BuildInfo
	}{
		{
			title:       "compile k6 v0.1.0",
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
			title:       "compile k6 missing version (v0.3.0)",
			k6Version:   "v0.3.0",
			mods:        []Module{},
			expectError: ErrResolvingDependency,
		},
		{
			title:       "compile k6 latest",
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
			title:     "compile k6 v0.1.0 with k6ext v0.1.0",
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
			title:     "compile k6 v0.1.0 with missing k6ext (v0.2.0)",
			k6Version: "v0.2.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", Version: "v0.2.0"},
			},
			expectError: ErrResolvingDependency,
		},
		{
			title:     "compile k6 v0.2.0 with k6extV2 (v2.0.0)",
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
			title:     "compile k6 v0.2.0 replace k6ext with local module",
			k6Version: "v0.2.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", ReplacePath: filepath.FromSlash("./testdata/mods/k6ext")},
			},
			expectError: nil,
			expect: &BuildInfo{
				Platform: "linux/amd64",
				ModVersions: map[string]string{
					"go.k6.io/k6":    "v0.2.0",
					"go.k6.io/k6ext": "v0.0.0-00010101000000-000000000000",
				},
			},
		},
		{
			title:     "compile k6 v0.2.0 replace k6ext with missing local module",
			k6Version: "v0.2.0",
			mods: []Module{
				// use FromSlash because Join removes the leading "."
				{Path: "go.k6.io/k6ext", ReplacePath: filepath.FromSlash("./testdata/mods/missing/k6ext")},
			},
			expectError: ErrResolvingDependency,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			platform, _ := ParsePlatform("linux/amd64")
			opts := NativeBuilderOpts{
				Stdout: os.Stdout,
				Stderr: os.Stderr,
				GoOpts: GoOpts{
					CopyGoEnv: true,
					Env: map[string]string{
						"GOPROXY":   goproxySrv.URL,
						"GONOPROXY": "none",
						"GOPRIVATE": "go.k6.io",
						"GONOSUMDB": "go.k6.io",
					},
					TmpCache: true,
				},
			}

			b, err := NewNativeBuilder(context.Background(), opts)
			if err != nil {
				t.Fatalf("setting up test %v", err)
			}

			outFile := &bytes.Buffer{}
			buildInfo, err := b.Build(
				context.Background(),
				platform,
				tc.k6Version,
				tc.mods,
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
