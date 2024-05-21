package main

import (
	"errors"
	"os"

	"github.com/grafana/k6build/pkg/builder"
	"github.com/spf13/cobra"
)

var ErrTargetPlatformUndefined = errors.New("target platform is required")

const example = `
k6build build -k v0.50.0 -d github.com/mostafa/xk6-kafka@v0.17.0
`

func newBuildCmd() *cobra.Command {

	var (
		deps []string
		k6Version string
		platformFlag string
		outPath string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short:  "build a custom k6 binary with extensions",
		Example: example,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			var err error
			platform := builder.RuntimePlatform()
			if platformFlag != "" {
				platform, err = builder.ParsePlatform(platformFlag)
				if err != nil {
					return err
				}
			}
	
			mods := []builder.Module{}
			for _, d := range deps {
				mod, err := builder.ParseModule(d)
				if err != nil {
					return err
				}
				mods = append(mods, mod)
			}

			b, err := builder.NewDefaultBuilder(ctx)
			if err != nil {
				return err
			}

			//TODO: check file permissions
			outFile, err := os.OpenFile(outPath, os.O_WRONLY | os.O_CREATE, 0777)
			if err != nil {
				return err
			}

			defer outFile.Close()

			err = b.Build(ctx, platform, k6Version, mods, outFile)

			return err
		},
	}

	cmd.Flags().StringArrayVarP(
		&deps,
		"dependency",
		"d",
		[]string{},
		"list of dependencies specifying the go mod path and version." +
		" If version is omitted, 'latest' is used.",
	)
	cmd.Flags().StringVarP(&k6Version, "k6-version", "k", "latest", "k6 version")
	cmd.Flags().StringVarP(&platformFlag, "platform", "p", "", "target platform in the format os/arch")
	cmd.Flags().StringVarP(&outPath, "output", "o", "k6", "path to output file")
	return cmd
}
