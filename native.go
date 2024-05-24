//nolint:forbidigo,revive
package k6build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
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

var (
	moduleVersionRegexp = regexp.MustCompile(`.+/v(\d+)$`)

	ErrNoGoToolchain = errors.New("go toolchain notfound")
	ErrNoGit         = errors.New("git notfound")
)

type nativeBuilder struct {
	opts      BuildOpts
	stderr    *os.File
	logWriter *io.PipeWriter
	logFlags  int
	logOutput io.Writer
}

type GoOpts struct {
	CopyEnv      bool
	Cgo          bool
	GoCache      string
	GoModCache   string
	TimeoutGet   time.Duration
	TimeoutBuild time.Duration
	RaceDetector bool
}

type BuildOpts struct {
	GoOpts
	K6Repo      string
	SkipCleanup bool
}

type goModTemplateContext struct {
	K6Module string
	Imports  []string
}

func NewNativeBuilder(_ context.Context, opts BuildOpts) (Builder, error) {
	if _, hasGo := goVersion(); !hasGo {
		return nil, ErrNoGoToolchain
	}

	if !hasGit() {
		return nil, ErrNoGit
	}

	return &nativeBuilder{
		opts: opts,
	}, nil
}

// Build builds a custom k6 binary for a target platform with the given dependencies into the out io.Writer
func (b *nativeBuilder) Build(
	ctx context.Context,
	platform Platform,
	k6Version string,
	mods []Module,
	out io.Writer,
) error {
	//TODO: move log setup out of build
	b.logFlags = log.Flags()
	b.logOutput = log.Writer()
	b.logWriter = logrus.StandardLogger().WriterLevel(logrus.DebugLevel)
	b.stderr = os.Stderr

	log.SetOutput(b.logWriter)
	log.SetFlags(0)

	workDir, err := os.MkdirTemp(os.TempDir(), defaultWorkDir)
	if err != nil {
		return fmt.Errorf("creating working directory: %w", err)
	}

	defer func() {
		if b.opts.SkipCleanup {
			log.Printf("[INFO] Skipping cleanup; leaving folder intact: %s", workDir)
			return
		}
		log.Printf("[INFO] Cleaning up work directory: %s", workDir)
		_ = os.RemoveAll(workDir)
	}()

	defer b.close()

	logrus.Debug("Building new k6 binary (native)")

	// prepare the build environment

	k6Path := filepath.Join(workDir, "k6")

	buildEnv, err := newGoEnv(
		workDir,
		b.opts.GoOpts,
		platform,
		//TODO: allow redirecting output
		os.Stdout,
		os.Stderr,
	)

	if err != nil {
		return err
	}

	log.Println("[INFO] Initializing Go module")
	err = buildEnv.modInit(ctx)
	if err != nil {
		return err
	}

	log.Println("[INFO] Creating k6 main")
	err = buildEnv.createMain(ctx, workDir, k6Version)
	if err != nil {
		return err
	}

	log.Println("[INFO] Updating modules")
	err = buildEnv.addMods(ctx, workDir, mods)
	if err != nil {
		return err
	}

	log.Println("[INFO] Building k6")

	err = buildEnv.compile(ctx, k6Path)
	if err != nil {
		return err
	}

	log.Printf("[INFO] Build complete")

	k6File, err := os.Open(k6Path)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, k6File)
	if err != nil {
		return fmt.Errorf("copying binary %w", err)
	}

	return nil
}

func (e *goEnv) createMain(ctx context.Context, path string, k6Version string) error {
	k6ModulePath, err := versionedModulePath(defaultK6ModulePath, k6Version)
	if err != nil {
		return err
	}

	// write the main module file
	mainPath := filepath.Join(path, "main.go")
	mainContent := fmt.Sprintf(mainModuleTemplate, k6ModulePath)
	err = os.WriteFile(mainPath, []byte(mainContent), 0o600)
	if err != nil {
		return fmt.Errorf("writing file %w", err)
	}

	err = e.modRequire(ctx, k6ModulePath, k6Version)
	if err != nil {
		return err
	}

	return e.modTidy(ctx)
}

// TODO: use golang.org/x/mod/modfile package to manipulate the gomod programmatically
func (e *goEnv) addMods(ctx context.Context, path string, mods []Module) error {
	for _, m := range mods {
		// write the module file
		modPath, err := versionedModulePath(m.PackagePath, m.Version)
		if err != nil {
			return err
		}

		modImportFile := filepath.Join(path, strings.ReplaceAll(modPath, "/", "_")+".go")
		modImportContent := fmt.Sprintf(modImportTemplate, modPath)
		err = os.WriteFile(modImportFile, []byte(modImportContent), 0o600)
		if err != nil {
			return fmt.Errorf("writing file %w", err)
		}

		err = e.modRequire(ctx, modPath, m.Version)
		if err != nil {
			return err
		}

		err = e.modTidy(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *nativeBuilder) close() {
	_ = b.logWriter.Close()

	log.SetFlags(b.logFlags)
	log.SetOutput(b.logOutput)

	os.Stderr = b.stderr
}

func goVersion() (string, bool) {
	cmd, err := exec.LookPath("go")
	if err != nil {
		return "", false
	}

	out, err := exec.Command(cmd, "version").Output() //nolint:gosec
	if err != nil {
		return "", false
	}

	pre := []byte("go")

	fields := bytes.SplitN(out, []byte{' '}, 4)
	if len(fields) < 4 || !bytes.Equal(fields[0], pre) || !bytes.HasPrefix(fields[2], pre) {
		return "", false
	}

	ver := string(bytes.TrimPrefix(fields[2], pre))

	return ver, true
}

func hasGit() bool {
	cmd, err := exec.LookPath("git")
	if err != nil {
		return false
	}

	_, err = exec.Command(cmd, "version").Output() //nolint:gosec

	return err == nil
}

// returns the modulePath with the major component of moduleVersion added,
// if it is a valid semantic version and is > 1
// Examples 
//  path="foo" and version="v1.0.0" returns "foo"
//  path="foo" and version="v2.0.0" returns "foo/v2"
//  path="foo/v2" and version="v3.0.0" returns an error
//  path="foo" and version="latest" returns "foo"
func versionedModulePath(modulePath, moduleVersion string) (string, error) {
	// if not is a semantic version return (could have been a commit SHA or 'latest')
	if !semver.IsValid(moduleVersion) {
		return modulePath, nil
	}
	major := semver.Major(moduleVersion)

	// if the module path has a major version at the end, check for inconsistencies
	if moduleVersionRegexp.MatchString(modulePath) {
		modPathVer:= filepath.Base(modulePath)
		if modPathVer != major {
			return "", fmt.Errorf("versioned module path %q and requested major version (%s) conflicts", modulePath, major)
		}
		return modulePath, nil
	}

	// if module path does not specify major version, add it if > 1
	switch major {
	case "v0", "v1":
		return modulePath, nil
	default:
		return filepath.Join(modulePath, major), nil
	}
}
