//nolint:forbidigo,gosec
package k6build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

const verInfoTemplate = "{\"Version\":\"%s\",\"Time\":\"%s\"}"

// adds a module version to the mod cache from a source directory, in a way that makes it
// compatible with the GOPROXY protocol (https://go.dev/ref/mod#goproxy-protocol)
// Creates the zip file, the version info and adds it to the version to the list file.
// Also updates the latest version.
// Example: for module go.k6.io/k6 version v0.1.0
//   - creates the directory ${cacheDir}/go.k6.io/k6/@v
//   - creates file v0.1.0.info
//   - compresses the module's source from ${src} into the file v0.1.0.zip
//   - copies the mod file from ${src} into v0.1.0.mod
func addModVersion(
	path string,
	version string,
	src string,
	cacheDir string,
) error {
	modPath := filepath.Join(cacheDir, path, "@v")
	err := os.MkdirAll(modPath, 0o755)
	if err != nil {
		return fmt.Errorf("creating module dir: %w", err)
	}

	// create zip file
	zipBuffer := &bytes.Buffer{}
	err = zip.CreateFromDir(zipBuffer, module.Version{Path: path, Version: version}, src)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}

	zipFile := filepath.Join(modPath, version+".zip")
	err = os.WriteFile(zipFile, zipBuffer.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	// create version info
	infoFile := filepath.Join(modPath, version+".info")
	verInfo := fmt.Sprintf(verInfoTemplate, version, time.Now().Format(time.RFC3339))
	err = os.WriteFile(infoFile, []byte(verInfo), 0o644)
	if err != nil {
		return fmt.Errorf("creating info file: %w", err)
	}

	// copy mod file
	buff, err := os.ReadFile(filepath.Join(src, "go.mod"))
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}
	modFile := filepath.Join(modPath, version+".mod")
	err = os.WriteFile(modFile, buff, 0o644)
	if err != nil {
		return fmt.Errorf("creating mod file: %w", err)
	}

	// update list of versions
	versionFiles, err := filepath.Glob(filepath.Join(modPath, "*.info"))
	if err != nil {
		return fmt.Errorf("listing versions: %w", err)
	}
	slices.Sort(versionFiles)

	list := []string{}
	for _, file := range versionFiles {
		ver, _ := strings.CutSuffix(file, ".info")
		list = append(list, ver)
	}

	listFile := filepath.Join(modPath, "list")
	err = os.WriteFile(listFile, []byte(strings.Join(list, "\n")), 0o644)
	if err != nil {
		return fmt.Errorf("creating list file: %w", err)
	}

	// update the latest version
	latestFile := filepath.Join(cacheDir, path, "@latest")
	latestVersion, err := os.ReadFile(latestFile)
	if errors.Is(err, os.ErrNotExist) || version > string(latestVersion) {
		err = os.WriteFile(latestFile, []byte(verInfo), 0o644)
		if err != nil {
			return fmt.Errorf("writing latest version: %w", err)
		}
	}

	return nil
}

func TestBuild(t *testing.T) {
	t.Parallel()

	// create modules for tests
	modules := []struct {
		source  string
		path    string
		version string
	}{
		{
			source:  "test/mods/k6",
			path:    "go.k6.io/k6",
			version: "v0.1.0",
		},
		{
			source:  "test/mods/k6",
			path:    "go.k6.io/k6",
			version: "v0.2.0",
		},
		{
			source:  "test/mods/k6ext",
			path:    "go.k6.io/k6ext",
			version: "v0.1.0",
		},
		{
			source:  "test/mods/k6extV2",
			path:    "go.k6.io/k6ext/v2",
			version: "v2.0.0",
		},
	}

	goproxy := filepath.Join(t.TempDir(), "goproxy")
	_ = os.Mkdir(goproxy, 0o777)

	for _, m := range modules {
		err := addModVersion(m.path, m.version, m.source, goproxy)
		if err != nil {
			t.Fatalf("setup %v", err)
		}
	}

	// create mod cache
	modcache := filepath.Join(t.TempDir(), "modcache")
	_ = os.Mkdir(modcache, 0o777)

	// deleting the modcache dir would fail because files are write protected, so we must
	// use go clean command
	t.Cleanup(func() {
		c := exec.Command("go", "clean", "-modcache")
		c.Env = []string{"GOMODCACHE=" + modcache}
		_ = c.Run()
	})

	testCases := []struct {
		title       string
		k6Version   string
		mods        []Module
		expectError error
	}{
		{
			title:       "compile k6 v0.1.0",
			k6Version:   "v0.1.0",
			mods:        []Module{},
			expectError: nil,
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
		},
		{
			title:     "compile k6 v0.1.0 with k6ext v0.1.0",
			k6Version: "v0.1.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", Version: "v0.1.0"},
			},
			expectError: nil,
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
			title:     "compile k6 v0.2.0 with k6extV2 (v0.2.0)",
			k6Version: "v0.2.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext/v2", Version: "v2.0.0"},
			},
			expectError: nil,
		},
		{
			title:     "compile k6 v0.2.0 replace k6extV2 with local module",
			k6Version: "v0.2.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", ReplacePath: "./test/mods/k6ext"},
			},
			expectError: nil,
		},
		{
			title:     "compile k6 v0.2.0 replace k6extV2 with missing local module",
			k6Version: "v0.2.0",
			mods: []Module{
				{Path: "go.k6.io/k6ext", ReplacePath: "./test/mods/mising/k6ext"},
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
				GoOpts: GoOpts{
					CopyEnv:    true,
					GoProxy:    fmt.Sprintf("file://%s", goproxy),
					GoNoProxy:  "none",
					GoPrivate:  "go.k6.io",
					GoModCache: modcache,
				},
			}

			b, err := NewNativeBuilder(context.Background(), opts)
			if err != nil {
				t.Fatalf("setting up test %v", err)
			}

			outFile := &bytes.Buffer{}
			err = b.Build(
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

			if tc.expectError == nil && outFile.Len() == 0 {
				t.Fatal("out file is empty")
			}
		})
	}
}
