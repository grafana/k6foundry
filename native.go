//nolint:forbidigo,revive,funlen
package k6foundry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultK6ModulePath = "go.k6.io/k6"

	defaultWorkDir = "k6foundry*"

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
	log *slog.Logger
}

// NativeBuilderOpts defines the options for the Native build environment
type NativeBuilderOpts struct {
	// options used for running go
	GoOpts
	// use alternative k6 repository
	K6Repo string
	// don't cleanup work environment (useful for debugging)
	SkipCleanup bool
	// redirect stdout
	Stdout io.Writer
	// redirect stderr
	Stderr io.Writer
	// set log level (INFO, WARN, ERROR)
	Logger *slog.Logger
}

// NewDefaultNativeBuilder creates a new native build environment with default options
func NewDefaultNativeBuilder() (Builder, error) {
	return NewNativeBuilder(
		context.TODO(),
		NativeBuilderOpts{
			GoOpts: GoOpts{
				CopyGoEnv: true,
			},
		},
	)
}

// NewNativeBuilder creates a new native build environment with the given options
func NewNativeBuilder(_ context.Context, opts NativeBuilderOpts) (Builder, error) {
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	// set default logger if none passed
	log := opts.Logger
	if log == nil {
		log = slog.New(
			slog.NewTextHandler(
				opts.Stderr,
				&slog.HandlerOptions{},
			),
		)
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
	buildOpts []string,
	binary io.Writer,
) error {
	workDir, err := os.MkdirTemp(os.TempDir(), defaultWorkDir)
	if err != nil {
		return fmt.Errorf("creating working directory: %w", err)
	}

	defer func() {
		if b.SkipCleanup {
			b.log.Info(fmt.Sprintf("Skipping cleanup. leaving directory %s intact", workDir))
			return
		}

		b.log.Info(fmt.Sprintf("Cleaning up work directory %s", workDir))
		_ = os.RemoveAll(workDir)
	}()

	// prepare the build environment
	b.log.Info("Building new k6 binary (native)")

	k6Binary := filepath.Join(workDir, "k6")

	buildEnv, err := newGoEnv(
		workDir,
		b.GoOpts,
		platform,
		b.Stdout,
		b.Stderr,
	)
	if err != nil {
		return err
	}

	defer func() {
		if b.SkipCleanup {
			b.log.Info("Skipping go cleanup")
			return
		}
		_ = buildEnv.close(ctx)
	}()

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

	k6Mod := Module{
		Path:        defaultK6ModulePath,
		Version:     k6Version,
		ReplacePath: b.K6Repo,
	}

	err = b.addMod(ctx, buildEnv, k6Mod)
	if err != nil {
		return err
	}

	b.log.Info("importing extensions")
	for _, m := range exts {
		err = b.createModuleImport(ctx, workDir, m)
		if err != nil {
			return err
		}

		err = b.addMod(ctx, buildEnv, m)
		if err != nil {
			return err
		}
	}

	b.log.Info("Building k6")
	err = buildEnv.compile(ctx, k6Binary, buildOpts...)
	if err != nil {
		return err
	}

	b.log.Info("Build complete")
	k6File, err := os.Open(k6Binary) //nolint:gosec
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

func (b *nativeBuilder) addMod(ctx context.Context, e *goEnv, mod Module) (err error) {
	defer func() {
		if err == nil {
			err = e.modTidy(ctx)
		}
	}()

	b.log.Info(fmt.Sprintf("adding dependency %s", mod.String()))

	if mod.ReplacePath == "" {
		return e.modRequire(ctx, mod.Path, mod.Version)
	}

	// resolve path to and absolute path because the mod replace will occur in the work directory
	replacePath, err := resolvePath(mod.ReplacePath)
	if err != nil {
		return fmt.Errorf("resolving replace path: %w", err)
	}
	return e.modReplace(ctx, mod.Path, mod.Version, replacePath, mod.ReplaceVersion)
}

func resolvePath(path string) (string, error) {
	var err error
	// expand environment variables
	if strings.Contains(path, "$") {
		path = os.ExpandEnv(path)
	}

	if strings.HasPrefix(path, ".") {
		path, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}
	}

	return path, nil
}

func (b *nativeBuilder) createModuleImport(_ context.Context, path string, mod Module) error {
	modImportFile := filepath.Join(path, strings.ReplaceAll(mod.Path, "/", "_")+".go")
	modImportContent := fmt.Sprintf(modImportTemplate, mod.Path)
	err := os.WriteFile(modImportFile, []byte(modImportContent), 0o600)
	if err != nil {
		return fmt.Errorf("writing mod file %w", err)
	}

	return nil
}
