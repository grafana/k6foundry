package cmd

import (
	"fmt"

	"go.k6.io/k6/v2/internal/build"
)

func Execute() {
	fmt.Printf(
		"build_origin=%s from_goflags=%s from_args=%s\n",
		build.BuildOrigin, build.FromGoflags, build.FromArgs,
	)
}
