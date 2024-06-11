package k6build

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var ErrTargetPlatformUndefined = errors.New("target platform is required") //nolint:revive

const long = `
builds a custom k6 binary with extensions.

The extensions are specified using the go module format: path[@version][replace@version]

The module's path must follow go conventions (e.g. github.com/my-module)
If version is omitted, 'latest' is used.
The replace path can be a mod path or a relative path (e.g. ../my-module).
If a relative replacement path is specified, the replacement version cannot be specified.
`

const example = `
# build k6 v0.50.0 with latest version of xk6-kubernetes
k6build -v v0.50.0 -d github.com/grafana/xk6-kubernetes

# build k6 v0.49.0 with xk6-kubernetes v0.9.0 and k6-output-kafka v0.7.0
k6build -v v0.49.0 -d github.com/grafana/xk6-kubernetes \
    -d github.com/grafana/xk6-output-kafka@v0.7.0

# build latest version of k6 with latest version of xk6-kubernetes v0.8.0
k6build -d github.com/grafana/xk6-kubernetes@v0.8.0

# build latest version of k6 and replace xk6-kubernetes with a local module
k6build -d github.com/grafana/xk6-kubernetes=../xk6-kubernetes

# build k6 from a local repository
k6build -r ../k6
`

// NewCmd creates new cobra command for build command.
func NewCmd() *cobra.Command {
	var (
		opts         NativeBuilderOpts
		deps         []string
		k6Version    string
		k6Repo       string
		platformFlag string
		outPath      string
		buildOpts    []string
	)

	cmd := &cobra.Command{
		Use:     "k6build",
		Short:   "build a custom k6 binary with extensions",
		Long:    long,
		Example: example,
		// prevent the usage help to printed to stderr when an error is reported by a subcommand
		SilenceUsage: true,
		// this is needed to prevent cobra to print errors reported by subcommands in the stderr
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			var err error
			platform := RuntimePlatform()
			if platformFlag != "" {
				platform, err = ParsePlatform(platformFlag)
				if err != nil {
					return err
				}
			}

			mods := []Module{}
			for _, d := range deps {
				mod, err2 := ParseModule(d)
				if err2 != nil {
					return err2
				}
				mods = append(mods, mod)
			}

			// set builder's output
			opts.Stdout = os.Stdout //nolint:forbidigo
			opts.Stderr = os.Stderr //nolint:forbidigo

			opts.K6Repo = k6Repo

			b, err := NewNativeBuilder(ctx, opts)
			if err != nil {
				return err
			}

			// TODO: check file permissions
			outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE, 0o777) //nolint:forbidigo,gosec
			if err != nil {
				return err
			}

			defer outFile.Close() //nolint:errcheck
			err = b.Build(ctx, platform, k6Version, mods, buildOpts, outFile)

			return err
		},
	}

	cmd.Flags().StringArrayVarP(
		&deps,
		"dependency",
		"d",
		[]string{},
		"list of dependencies using go mod format: path[@version][replace@version]",
	)
	cmd.Flags().StringVarP(&k6Version, "k6-version", "v", "latest", "k6 version")
	cmd.Flags().StringVarP(&k6Repo, "k6-repostitory", "r", "", "k6 repository")
	cmd.Flags().StringVarP(&platformFlag, "platform", "p", "", "target platform in the format os/arch")
	cmd.Flags().StringVarP(&outPath, "output", "o", "k6", "path to output file")
	cmd.Flags().BoolVar(&opts.CopyEnv, "copy-env", false, "copy current environment variables")
	cmd.Flags().StringVar(&opts.LogLevel, "log-level", "error", "log level")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "verbose build output")
	cmd.Flags().StringArrayVarP(&buildOpts, "build-opts", "b", []string{}, "go build opts. e.g. -ldflags='-w -s'")
	return cmd
}
