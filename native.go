//nolint:forbidigo,revive
package k6build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	"go.k6.io/xk6"
)

var (
	ErrNoGoToolchain = errors.New("go toolchain notfound")
	ErrNoGit         = errors.New("git notfound")
)

type nativeBuilder struct {
	stderr    *os.File
	logWriter *io.PipeWriter
	logFlags  int
	logOutput io.Writer
}

func newNativeBuilder(_ context.Context) (Builder, error) {
	if _, hasGo := goVersion(); !hasGo {
		return nil, ErrNoGoToolchain
	}

	if !hasGit() {
		return nil, ErrNoGit
	}

	return new(nativeBuilder), nil
}

// Build builds a custom k6 binary for a target platform with the given dependencies into the out io.Writer
func (b *nativeBuilder) Build(
	ctx context.Context,
	platform Platform,
	k6Version string,
	mods []Module,
	out io.Writer,
) error {
	b.logFlags = log.Flags()
	b.logOutput = log.Writer()
	b.logWriter = logrus.StandardLogger().WriterLevel(logrus.DebugLevel)
	b.stderr = os.Stderr

	log.SetOutput(b.logWriter)
	log.SetFlags(0)

	if null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stderr = null
	}

	defer b.close()

	logrus.Debug("Building new k6 binary (native)")

	xk6Builder := new(xk6.Builder)

	xk6Builder.Cgo = false
	xk6Builder.OS = platform.OS
	xk6Builder.Arch = platform.Arch
	xk6Builder.K6Version = k6Version

	for _, m := range mods {
		xk6Builder.Extensions = append(xk6Builder.Extensions,
			xk6.Dependency{
				PackagePath: m.PackagePath,
				Version:     m.Version,
			},
		)
	}

	tmp, err := os.CreateTemp("", "k6")
	if err != nil {
		return err
	}

	if err = tmp.Close(); err != nil {
		return err
	}

	if err = xk6Builder.Build(ctx, tmp.Name()); err != nil {
		return err
	}

	tmp, err = os.Open(tmp.Name())
	if err != nil {
		return err
	}

	_, err = io.Copy(out, tmp)

	tmp.Close()           //nolint:errcheck,gosec
	os.Remove(tmp.Name()) //nolint:errcheck,gosec

	return err
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
