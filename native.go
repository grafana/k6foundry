//nolint:forbidigo,revive,funlen
package k6build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	defaultK6ModulePath = "go.k6.io/k6"

	defaultWorkDir = "k6build*"

	mainModuleTemplate = `package main

import (
	k6cmd "%s/cmd"

)

func main() {
	k6cmd.Execute()
}
`
	modImportTemplate = `package main

	import _ %q
`
)

type nativeBuilder struct {
	NativeBuilderOpts
	log *logrus.Logger
}

// NativeBuilderOpts defines the options for the Native build environment
type NativeBuilderOpts struct {
	GoOpts
	K6Repo      string
	SkipCleanup bool
	Stdout      io.Writer
	Stderr      io.Writer
	LogLevel    string
	Verbose     bool
}

// NewNativeBuilder creates a new native build environment with the given options
func NewNativeBuilder(_ context.Context, opts NativeBuilderOpts) (Builder, error) {
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	var err error
	logLevel := logrus.ErrorLevel
	if opts.LogLevel != "" {
		logLevel, err = logrus.ParseLevel(opts.LogLevel)
		if err != nil {
			return nil, fmt.Errorf("parsing log level %w", err)
		}
	}
	log := &logrus.Logger{
		Out:       opts.Stderr,
		Formatter: new(logrus.TextFormatter),
		Level:     logLevel,
	}

	return &nativeBuilder{
		NativeBuilderOpts: opts,
		log:               log,
	}, nil
}

// Build builds a custom k6 binary for a target platform with the given dependencies into the out io.Writer
func (b *nativeBuilder) Build(
	ctx context.Context,
	platform Platform,
	k6Version string,
	exts []Module,
	binary io.Writer,
) error {
	workDir, err := os.MkdirTemp(os.TempDir(), defaultWorkDir)
	if err != nil {
		return fmt.Errorf("creating working directory: %w", err)
	}

	defer func() {
		if b.SkipCleanup {
			b.log.Infof("Skipping cleanup; leaving folder intact: %s", workDir)
			return
		}
		b.log.Infof("Cleaning up work directory: %s", workDir)
		_ = os.RemoveAll(workDir)
	}()

	// prepare the build environment
	b.log.Info("Building new k6 binary (native)")

	k6Path := filepath.Join(workDir, "k6")

	goOut := io.Discard
	goErr := io.Discard
	if b.Verbose {
		goOut = b.Stdout
		goErr = b.Stderr
	}
	buildEnv, err := newGoEnv(
		workDir,
		b.GoOpts,
		platform,
		goOut,
		goErr,
	)
	if err != nil {
		return err
	}

	b.log.Info("Initializing Go module")
	err = buildEnv.modInit(ctx)
	if err != nil {
		return err
	}

	b.log.Info("Creating k6 main")
	err = b.createMain(ctx, workDir)
	if err != nil {
		return err
	}

	err = buildEnv.addMod(ctx, defaultK6ModulePath, k6Version)
	if err != nil {
		return err
	}

	b.log.Info("importing extensions")
	for _, m := range exts {
		err = b.createModuleImport(ctx, workDir, m)
		if err != nil {
			return err
		}

		err = buildEnv.addMod(ctx, m.PackagePath, m.Version)
		if err != nil {
			return err
		}
	}

	b.log.Info("Building k6")
	err = buildEnv.compile(ctx, k6Path)
	if err != nil {
		return err
	}

	b.log.Info("Build complete")
	k6File, err := os.Open(k6Path) //nolint:gosec
	if err != nil {
		return err
	}

	_, err = io.Copy(binary, k6File)
	if err != nil {
		return fmt.Errorf("copying binary %w", err)
	}

	return nil
}

func (b *nativeBuilder) createMain(_ context.Context, path string) error {
	// write the main module file
	mainPath := filepath.Join(path, "main.go")
	mainContent := fmt.Sprintf(mainModuleTemplate, defaultK6ModulePath)
	err := os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		return fmt.Errorf("writing main file %w", err)
	}

	return nil
}

func (b *nativeBuilder) createModuleImport(_ context.Context, path string, mod Module) error {
	modImportFile := filepath.Join(path, strings.ReplaceAll(mod.PackagePath, "/", "_")+".go")
	modImportContent := fmt.Sprintf(modImportTemplate, mod.PackagePath)
	err := os.WriteFile(modImportFile, []byte(modImportContent), 0o600)
	if err != nil {
		return fmt.Errorf("writing mod file %w", err)
	}

	return nil
}
