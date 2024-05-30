//nolint:revive
package k6build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

var (
	ErrCompiling           = errors.New("compiling")
	ErrExecutingGoCommand  = errors.New("executing go command")
	ErrNoGoToolchain       = errors.New("go toolchain notfound")
	ErrNoGit               = errors.New("git notfound")
	ErrResolvingDependency = errors.New("resolving dependency")
	ErrSettingGoEnv        = errors.New("setting go environment")
)

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
	if _, hasGo := goVersion(); !hasGo {
		return nil, ErrNoGoToolchain
	}

	if !hasGit() {
		return nil, ErrNoGit
	}

	env := map[string]string{}
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

	cmdEnv := []string{}
	if opts.CopyEnv {
		cmdEnv = os.Environ() //nolint:forbidigo
	}
	return &goEnv{
		env:      append(cmdEnv, mapToSlice(env)...),
		platform: platform,
		opts:     opts,
		workDir:  workDir,
		stdout:   stdout,
		stderr:   stderr,
	}, nil
}

// TODO: use golang.org/x/mod/modfile package to manipulate the gomod programmatically
func (e *goEnv) addMod(ctx context.Context, path string, version string) error {
	if err := e.modRequire(ctx, path, version); err != nil {
		return err
	}
	return e.modTidy(ctx)
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
		return fmt.Errorf("%w: %s", ErrExecutingGoCommand, err) //nolint:errorlint
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
			cmdErr = fmt.Errorf("%w: %s", ErrExecutingGoCommand, err) //nolint:errorlint
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
		return fmt.Errorf("%w: %s", ErrSettingGoEnv, err) //nolint:errorlint
	}

	return nil
}

// tidy the module to ensure go.mod will not have versions such as `latest`
func (e goEnv) modTidy(ctx context.Context) error {
	err := e.runGo(ctx, e.opts.TimeoutGet, "mod", "tidy", "-compat=1.17")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrResolvingDependency, err) //nolint:errorlint
	}

	return nil
}

func (e goEnv) modRequire(ctx context.Context, modulePath, moduleVersion string) error {
	mod := modulePath
	if moduleVersion != "" {
		mod += "@" + moduleVersion
	} else {
		mod += "@latest"
	}
	err := e.runGo(ctx, e.opts.TimeoutGet, "mod", "edit", "-require", mod)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrResolvingDependency, err) //nolint:errorlint
	}

	return nil
}

func (e goEnv) compile(ctx context.Context, outPath string) error {
	buildFlags := []string{
		"-o", outPath,
		//	"-ldflags=\"-w -s\"",
		"-trimpath",
	}
	if e.opts.RaceDetector {
		buildFlags = append(buildFlags, "-race")
		e.opts.Cgo = true
	}
	args := append([]string{"build"}, buildFlags...)
	err := e.runGo(ctx, e.opts.TimeoutGet, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrCompiling, err) //nolint:errorlint
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

func hasGit() bool {
	cmd, err := exec.LookPath("git")
	if err != nil {
		return false
	}

	_, err = exec.Command(cmd, "version").Output() //nolint:gosec

	return err == nil
}
