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

// adds a package version to the mod cache from a source directory, in a way that makes it
// compatible with the GOPROXY protocol (https://go.dev/ref/mod#goproxy-protocol)
// Creates the zip file, the version info and adds it to the version to the list file.
// Also updates the latest version.
// Example: for package go.k6.io/k6 version v0.1.0
//   - creates the directory ${cacheDir}/go.k6.io/k6/@v
//   - creates file v0.1.0.info
//   - compresses the module's source from ${pkgSrc} into the file v0.1.0.zip
//   - copies the mod file from ${pkgSrc} into v0.1.0.mod
func addPackageVersion(
	pkg string,
	pkgVersion string,
	pkgSrc string,
	cacheDir string,
) error {
	packagePath := filepath.Join(cacheDir, pkg, "@v")
	err := os.MkdirAll(packagePath, 0o755)
	if err != nil {
		return fmt.Errorf("creating package dir: %w", err)
	}

	// create zip file
	zipBuffer := &bytes.Buffer{}
	version := module.Version{Path: pkg, Version: pkgVersion}
	err = zip.CreateFromDir(zipBuffer, version, pkgSrc)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}

	zipFile := filepath.Join(packagePath, pkgVersion+".zip")
	err = os.WriteFile(zipFile, zipBuffer.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	// create version info
	infoFile := filepath.Join(packagePath, pkgVersion+".info")
	verInfo := fmt.Sprintf(verInfoTemplate, pkgVersion, time.Now().Format(time.RFC3339))
	err = os.WriteFile(infoFile, []byte(verInfo), 0o644)
	if err != nil {
		return fmt.Errorf("creating info file: %w", err)
	}

	// copy mod file
	mod, err := os.ReadFile(filepath.Join(pkgSrc, "go.mod"))
	if err != nil {
		return fmt.Errorf("reading go.mod: %w", err)
	}
	modFile := filepath.Join(packagePath, pkgVersion+".mod")
	err = os.WriteFile(modFile, mod, 0o644)
	if err != nil {
		return fmt.Errorf("creating mod file: %w", err)
	}

	// update list of versions
	versionFiles, err := filepath.Glob(filepath.Join(packagePath, "*.info"))
	if err != nil {
		return fmt.Errorf("listing versions: %w", err)
	}
	slices.Sort(versionFiles)

	list := []string{}
	for _, file := range versionFiles {
		version, _ := strings.CutSuffix(file, ".info")
		list = append(list, version)
	}

	listFile := filepath.Join(packagePath, "list")
	err = os.WriteFile(listFile, []byte(strings.Join(list, "\n")), 0o644)
	if err != nil {
		return fmt.Errorf("creating list file: %w", err)
	}

	// update the latest version
	latestFile := filepath.Join(cacheDir, pkg, "@latest")
	latestVersion, err := os.ReadFile(latestFile)
	if errors.Is(err, os.ErrNotExist) || pkgVersion > string(latestVersion) {
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
	pkgs := []struct {
		pkgSrc  string
		pkgPath string
		version string
	}{
		{
			pkgSrc:  "test/pkgs/k6",
			pkgPath: "go.k6.io/k6",
			version: "v0.1.0",
		},
		{
			pkgSrc:  "test/pkgs/k6",
			pkgPath: "go.k6.io/k6",
			version: "v0.2.0",
		},
		{
			pkgSrc:  "test/pkgs/k6ext",
			pkgPath: "go.k6.io/k6ext",
			version: "v0.1.0",
		},
		{
			pkgSrc:  "test/pkgs/k6extV2",
			pkgPath: "go.k6.io/k6ext/v2",
			version: "v2.0.0",
		},
	}

	goproxy := filepath.Join(t.TempDir(), "goproxy")
	_ = os.Mkdir(goproxy, 0o777)

	for _, pkg := range pkgs {
		err := addPackageVersion(pkg.pkgPath, pkg.version, pkg.pkgSrc, goproxy)
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
				{PackagePath: "go.k6.io/k6ext", Version: "v0.1.0"},
			},
			expectError: nil,
		},
		{
			title:     "compile k6 v0.1.0 with missing k6ext (v0.2.0)",
			k6Version: "v0.2.0",
			mods: []Module{
				{PackagePath: "go.k6.io/k6ext", Version: "v0.2.0"},
			},
			expectError: ErrResolvingDependency,
		},
		{
			title:     "compile k6 v0.1.0 with k6extV2 (v0.2.0)",
			k6Version: "v0.2.0",
			mods: []Module{
				{PackagePath: "go.k6.io/k6ext/v2", Version: "v2.0.0"},
			},
			expectError: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			platform, _ := ParsePlatform("linux/amd64")

			opts := BuildOpts{
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
