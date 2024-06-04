// Package cmd contains build cobra command factory function.
package cmd

import (
	"errors"
	"os"

	"github.com/grafana/k6build"
	"github.com/spf13/cobra"
)

var ErrTargetPlatformUndefined = errors.New("target platform is required") //nolint:revive

const example = `
k6build build -k v0.50.0 -d github.com/mostafa/xk6-kafka@v0.17.0
`

// New creates new cobra command for build command.
func New() *cobra.Command {
	var (
		deps         []string
		k6Version    string
		platformFlag string
		outPath      string
	)

	cmd := &cobra.Command{
		Use:     "build",
		Short:   "build a custom k6 binary with extensions",
		Example: example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			var err error
			platform := k6build.RuntimePlatform()
			if platformFlag != "" {
				platform, err = k6build.ParsePlatform(platformFlag)
				if err != nil {
					return err
				}
			}

			mods := []k6build.Module{}
			for _, d := range deps {
				mod, err2 := k6build.ParseModule(d)
				if err2 != nil {
					return err2
				}
				mods = append(mods, mod)
			}

			b, err := k6build.NewDefaultBuilder(ctx)
			if err != nil {
				return err
			}

			// TODO: check file permissions
			outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE, 0o777) //nolint:forbidigo,gosec
			if err != nil {
				return err
			}

			defer outFile.Close() //nolint:errcheck

			err = b.Build(ctx, platform, k6Version, mods, outFile)

			return err
		},
	}

	cmd.Flags().StringArrayVarP(
		&deps,
		"dependency",
		"d",
		[]string{},
		"list of dependencies specifying the go mod path and version."+
			" If version is omitted, 'latest' is used.",
	)
	cmd.Flags().StringVarP(&k6Version, "k6-version", "k", "latest", "k6 version")
	cmd.Flags().StringVarP(&platformFlag, "platform", "p", "", "target platform in the format os/arch")
	cmd.Flags().StringVarP(&outPath, "output", "o", "k6", "path to output file")
	return cmd
}
