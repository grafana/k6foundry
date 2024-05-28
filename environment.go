package k6build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
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
		cmdEnv = os.Environ()
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

func (e goEnv) newCommand(command string, args ...string) *exec.Cmd {
	cmd := exec.Command(command, args...)

	cmd.Env = e.env
	cmd.Dir = e.workDir
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	return cmd
}

func (e goEnv) runCommand(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// start the command; if it fails to start, report error immediately
	err := cmd.Start()
	if err != nil {
		return err
	}

	// wait for the command in a goroutine; the reason for this is
	// very subtle: if, in our select, we do `case cmdErr := <-cmd.Wait()`,
	// then that case would be chosen immediately, because cmd.Wait() is
	// immediately available (even though it blocks for potentially a long
	// time, it can be evaluated immediately). So we have to remove that
	// evaluation from the `case` statement.
	cmdErrChan := make(chan error)
	go func() {
		cmdErrChan <- cmd.Wait()
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
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
		case <-cmdErrChan:
		}
		return ctx.Err()
	}
}

func (e goEnv) modInit(ctx context.Context) error {
	// initialize the go module
	cmd := e.newCommand("go", "mod", "init", "k6")
	// TODO: change magic constant in timeout
	return e.runCommand(ctx, cmd, 10*time.Second)

}

// tidy the module to ensure go.mod will not have versions such as `latest`
func (e goEnv) modTidy(ctx context.Context) error {
	tidyCmd := e.newCommand("go", "mod", "tidy", "-compat=1.17")
	return e.runCommand(ctx, tidyCmd, e.opts.TimeoutGet)
}

func (e goEnv) modRequire(ctx context.Context, modulePath, moduleVersion string) error {
	mod := modulePath
	if moduleVersion != "" {
		mod += "@" + moduleVersion
	} else {
		mod += "@latest"
	}
	cmd := e.newCommand("go", "mod", "edit", "-require", mod)
	err := e.runCommand(ctx, cmd, e.opts.TimeoutGet)

	return err
}

func (e goEnv) modReplace(ctx context.Context, modulePath, replaceRepo string) error {
	replace := fmt.Sprintf("%s=%s", modulePath, replaceRepo)
	cmd := e.newCommand("go", "mod", "edit", "-replace", replace)
	err := e.runCommand(ctx, cmd, e.opts.TimeoutGet)
	if err != nil {
		return err
	}
	return e.modTidy(ctx)
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
	cmd := e.newCommand("go", args...)

	err := e.runCommand(ctx, cmd, e.opts.TimeoutGet)

	return err
}

func mapToSlice(m map[string]string) []string {
	s := []string{}
	for k, v := range m {
		s = append(s, fmt.Sprintf("%s=%s", k, v))
	}
	return s
}
