// Package goproxy implements a go proxy that resolves requests from an in memory go mood cache
package goproxy

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	infoTemplate = "{\"Version\":\"%s\",\"Time\":\"%s\"}"
)

// GoProxy implements a Go proxy.
// Uses a in memory representation of the mod cache to store a mod cache
// Responds to GOPROXY protocol requests from the mod cache
// See https://go.dev/ref/mod#goproxy-protocol)
type GoProxy struct {
	files    map[string][]byte
	versions map[string][]string
}

// NewGoProxy creates a new GoProxy
func NewGoProxy() *GoProxy {
	return &GoProxy{
		files:    map[string][]byte{},
		versions: map[string][]string{},
	}
}

// ServeHTTP handles GOPROXY requests
func (p *GoProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Path
	content, found := p.files[file]

	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch filepath.Ext(file) {
	case ".zip":
		w.Header().Add("Content-type", "application/zip")
	case ".info":
		w.Header().Add("Content-type", "application/json")
	default:
		w.Header().Add("Content-type", "text/plain")
	}

	_, _ = w.Write(content)
}

// AddModVersion adds a module version to the go proxy cache
// Example: for module go.k6.io/k6 version v0.1.0
//   - creates info file go.k6.io/k6/@v/v0.1.0.info
//   - copies the mod file from into go.k6.io/k6/@v/v0.1.0.mod
//   - compresses the gomod and source files into the file go.k6.io/k6/@v/v0.1.0.zip
//   - updates the list of versions at go.k6.io/k6/@v/list
//   - updates the latest version for the module at go.k6.io/k6/@latest
func (p *GoProxy) AddModVersion(
	path string,
	version string,
	sourcePath string,
) error {
	// create modules for tests
	sourceFiles, err := ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("reading module source: %w", err)
	}

	modPath := filepath.Join("/", path, "@v")

	// create gomod
	gomod, found := sourceFiles["go.mod"]
	if !found {
		return fmt.Errorf("go.mod is required")
	}

	// create zip file
	zipRoot := fmt.Sprintf("%s@%s", path, version)
	zipBuffer := &bytes.Buffer{}
	zipWriter := zip.NewWriter(zipBuffer)
	for name, content := range sourceFiles {
		w, err2 := zipWriter.Create(filepath.Join(zipRoot, name))
		if err2 != nil {
			return fmt.Errorf("creating zip file: %w", err2)
		}
		_, err2 = w.Write(content)
		if err2 != nil {
			return fmt.Errorf("creating zip file: %w", err2)
		}
	}
	err = zipWriter.Close()
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}

	zipFile := filepath.Join(modPath, version+".zip")
	p.files[zipFile] = zipBuffer.Bytes()

	// create version info
	infoFile := filepath.Join(modPath, version+".info")
	verInfo := fmt.Sprintf(infoTemplate, version, time.Now().Format(time.RFC3339))
	p.files[infoFile] = []byte(verInfo)

	// copy mod file
	modFile := filepath.Join(modPath, version+".mod")
	p.files[modFile] = gomod

	// update list of versions
	versions := p.versions[path]
	versions = append(versions, version)
	slices.Sort(versions)

	listFile := filepath.Join(modPath, "list")
	p.files[listFile] = []byte(strings.Join(versions, "\n"))

	// update the latest version
	latestFile := filepath.Join(path, "@latest")
	latestVersion := slices.Max(versions)
	p.files[latestFile] = []byte(latestVersion)

	return nil
}
