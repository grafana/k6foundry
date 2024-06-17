//nolint:revive,forbidigo
package k6foundry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

var (
	// Error compiling binary
	ErrCompiling = errors.New("compiling")
	// Error executing go command
	ErrExecutingGoCommand = errors.New("executing go command")
	// Go toolchacin is not installed
	ErrNoGoToolchain = errors.New("go toolchain notfound")
	// Git is not installed
	ErrNoGit = errors.New("git notfound")
	// Error resolving dependency
	ErrResolvingDependency = errors.New("resolving dependency")
	// Error initiailizing go build environment
	ErrSettingGoEnv = errors.New("setting go environment")
)

// GoOpts defines the options for the go build environment
type GoOpts struct {
	// Copy Environment variables to go build environment
	CopyGoEnv bool
	// Enable Cgo. Overrides values in environment if CopyEnv is true
	Cgo bool
	// Sets GONOSUMDB environment variables. Overrides values in environment if CopyEnv is true
	GoNoSumDB string
	// Sets GOCACHE. Overrides values in environment if CopyEnv is true
	GoCache string
	// sets GOMODCACHE
	GoModCache string
	// Sets GOPROXY. Overrides values in environment if CopyEnv is true
	GoProxy string
	// Sets GONOPROXY. Overrides values in environment if CopyEnv is true
	GoNoProxy string
	// Sets GOPRIVATE. Overrides values in environment if CopyEnv is true
	GoPrivate string
	// Timeout for getting modules
	GoGetTimeout time.Duration
	// Timeout for building binary
	GOBuildTimeout time.Duration
	// Use an ephemeral cache. Ignores GoModCache and GoCache
	EphemeralCache bool
}

type goEnv struct {
	env      []string
	workDir  string
	opts     GoOpts
	platform Platform
	stdout   io.Writer
	stderr   io.Writer
}

func newGoEnv(
	workDir string,
	opts GoOpts,
	platform Platform,
	stdout io.Writer,
	stderr io.Writer,
) (*goEnv, error) {
	var err error

	if _, hasGo := goVersion(); !hasGo {
		return nil, ErrNoGoToolchain
	}

	if !hasGit() {
		return nil, ErrNoGit
	}

	if opts.EphemeralCache {
		var modCache, goCache string
		// override caches with temporary files
		modCache, err = os.MkdirTemp(os.TempDir(), "modcache*")
		if err != nil {
			return nil, fmt.Errorf("creating mod cache %w", err)
		}

		goCache, err = os.MkdirTemp(os.TempDir(), "cache*")
		if err != nil {
			return nil, fmt.Errorf("creating go cache %w", err)
		}

		opts.GoCache = goCache
		opts.GoModCache = modCache
	}

	env := map[string]string{}
	if opts.CopyGoEnv {
		env, err = getGoEnv()
		if err != nil {
			return nil, fmt.Errorf("copying go environment %w", err)
		}
	}

	env["PATH"] = os.Getenv("PATH")

	env["GOOS"] = platform.OS
	env["GOARCH"] = platform.Arch
	env["CGO_ENABLED"] = fmt.Sprintf("%t", opts.Cgo)
	if opts.GoCache != "" {
		env["GOCACHE"] = opts.GoCache
	}
	if opts.GoModCache != "" {
		env["GOMODCACHE"] = opts.GoModCache
	}
	if opts.GoProxy != "" {
		env["GOPROXY"] = opts.GoProxy
	}
	if opts.GoNoProxy != "" {
		env["GONOPROXY"] = opts.GoNoProxy
	}
	if opts.GoPrivate != "" {
		env["GOPRIVATE"] = opts.GoPrivate
	}
	if opts.GoNoSumDB != "" {
		env["GONOSUMDB"] = opts.GoNoSumDB
	}

	return &goEnv{
		env:      mapToSlice(env),
		platform: platform,
		opts:     opts,
		workDir:  workDir,
		stdout:   stdout,
		stderr:   stderr,
	}, nil
}

func (e goEnv) close(ctx context.Context) error {
	if e.opts.EphemeralCache {
		return errors.Join(
			// clean caches otherwise can't delete the directories
			// because cached files are readonly
			e.clean(ctx),
			os.RemoveAll(e.opts.GoCache),
			os.RemoveAll(e.opts.GoModCache),
		)
	}

	return nil
}

func (e goEnv) runGo(ctx context.Context, timeout time.Duration, args ...string) error {
	cmd := exec.Command("go", args...)

	cmd.Env = e.env
	cmd.Dir = e.workDir

	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// start the command; if it fails to start, report error immediately
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrExecutingGoCommand, err.Error())
	}

	// wait for the command in a goroutine; the reason for this is
	// very subtle: if, in our select, we do `case cmdErr := <-cmd.Wait()`,
	// then that case would be chosen immediately, because cmd.Wait() is
	// immediately available (even though it blocks for potentially a long
	// time, it can be evaluated immediately). So we have to remove that
	// evaluation from the `case` statement.
	cmdErrChan := make(chan error)
	go func() {
		cmdErr := cmd.Wait()
		if cmdErr != nil {
			cmdErr = fmt.Errorf("%w: %s", ErrExecutingGoCommand, cmdErr.Error())
		}
		cmdErrChan <- cmdErr
	}()

	// unblock either when the command finishes, or when the done
	// channel is closed -- whichever comes first
	select {
	case cmdErr := <-cmdErrChan:
		// process ended; report any error immediately
		return cmdErr
	case <-ctx.Done():
		// context was canceled, either due to timeout or
		// maybe a signal from higher up canceled the parent
		// context; presumably, the OS also sent the signal
		// to the child process, so wait for it to die
		select {
		// TODO: check this magic timeout
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
		case <-cmdErrChan:
		}
		return ctx.Err()
	}
}

func (e goEnv) modInit(ctx context.Context) error {
	// initialize the go module
	// TODO: change magic constant in timeout
	err := e.runGo(ctx, 10*time.Second, "mod", "init", "k6")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSettingGoEnv, err.Error())
	}

	return nil
}

// tidy the module to ensure go.mod will not have versions such as `latest`
func (e goEnv) modTidy(ctx context.Context) error {
	err := e.runGo(ctx, e.opts.GoGetTimeout, "mod", "tidy", "-compat=1.17")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrResolvingDependency, err.Error())
	}

	return nil
}

func (e goEnv) modRequire(ctx context.Context, modulePath, moduleVersion string) error {
	if moduleVersion != "" {
		modulePath += "@" + moduleVersion
	}

	err := e.runGo(ctx, e.opts.GoGetTimeout, "mod", "edit", "-require", modulePath)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrResolvingDependency, err.Error())
	}

	return nil
}

func (e goEnv) modReplace(ctx context.Context, modulePath, moduleVersion, replacePath, replaceVersion string) error {
	if moduleVersion != "" {
		modulePath += "@" + moduleVersion
	}

	if replaceVersion != "" {
		replacePath += "@" + replaceVersion
	}

	err := e.runGo(ctx, e.opts.GoGetTimeout, "mod", "edit", "-replace", fmt.Sprintf("%s=%s", modulePath, replacePath))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrResolvingDependency, err.Error())
	}

	return nil
}

func (e goEnv) compile(ctx context.Context, outPath string, buildFlags ...string) error {
	args := append([]string{"build", "-o", outPath}, buildFlags...)

	err := e.runGo(ctx, e.opts.GOBuildTimeout, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrCompiling, err.Error())
	}

	return err
}

func (e goEnv) clean(ctx context.Context) error {
	err := e.runGo(ctx, e.opts.GOBuildTimeout, "clean", "-cache", "-modcache")
	if err != nil {
		return fmt.Errorf("cleaning: %s", err.Error())
	}

	return err
}

func mapToSlice(m map[string]string) []string {
	s := []string{}
	for k, v := range m {
		s = append(s, fmt.Sprintf("%s=%s", k, v))
	}

	return s
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

func getGoEnv() (map[string]string, error) {
	cmd, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("getting go binary %w", err)
	}

	out, err := exec.Command(cmd, "env", "-json").Output() //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("getting go env %w", err)
	}

	envMap := map[string]string{}

	err = json.Unmarshal(out, &envMap)
	if err != nil {
		return nil, fmt.Errorf("getting go env %w", err)
	}

	return envMap, err
}

func hasGit() bool {
	cmd, err := exec.LookPath("git")
	if err != nil {
		return false
	}

	_, err = exec.Command(cmd, "version").Output() //nolint:gosec

	return err == nil
}
