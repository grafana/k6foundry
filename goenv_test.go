package k6foundry

import (
	"io"
	"strings"
	"testing"
)

func TestNewGoEnvCGOEnabled(t *testing.T) {
	t.Parallel()

	host := RuntimePlatform()
	crossOS := "linux"
	if host.OS == "linux" {
		crossOS = "darwin"
	}
	crossPlatform, err := NewPlatform(crossOS, host.Arch)
	if err != nil {
		t.Fatalf("setup cross platform: %v", err)
	}

	testCases := []struct {
		title      string
		platform   Platform
		env        map[string]string
		wantCGO    string
		wantCGOSet bool
	}{
		{
			title:    "cross-build defaults CGO off",
			platform: crossPlatform,
			env: map[string]string{
				"GOHOSTOS":   host.OS,
				"GOHOSTARCH": host.Arch,
			},
			wantCGO:    "0",
			wantCGOSet: true,
		},
		{
			title:    "cross-build respects explicit CGO_ENABLED=1",
			platform: crossPlatform,
			env: map[string]string{
				"GOHOSTOS":    host.OS,
				"GOHOSTARCH":  host.Arch,
				"CGO_ENABLED": "1",
			},
			wantCGO:    "1",
			wantCGOSet: true,
		},
		{
			title:    "cross-build respects explicit CGO_ENABLED=0",
			platform: crossPlatform,
			env: map[string]string{
				"GOHOSTOS":    host.OS,
				"GOHOSTARCH":  host.Arch,
				"CGO_ENABLED": "0",
			},
			wantCGO:    "0",
			wantCGOSet: true,
		},
		{
			title:    "native build keeps explicit CGO_ENABLED=1",
			platform: host,
			env: map[string]string{
				"GOHOSTOS":    host.OS,
				"GOHOSTARCH":  host.Arch,
				"CGO_ENABLED": "1",
			},
			wantCGO:    "1",
			wantCGOSet: true,
		},
		{
			title:    "native build does not force CGO off",
			platform: host,
			env: map[string]string{
				"GOHOSTOS":   host.OS,
				"GOHOSTARCH": host.Arch,
			},
			wantCGOSet: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			ge, err := newGoEnv(
				t.TempDir(),
				GoOpts{CopyGoEnv: false, Env: tc.env},
				tc.platform,
				io.Discard,
				io.Discard,
			)
			if err != nil {
				t.Fatalf("newGoEnv: %v", err)
			}
			t.Cleanup(func() {
				_ = ge.close(t.Context())
			})

			got, ok := lookupEnv(ge.env, "CGO_ENABLED")
			if ok != tc.wantCGOSet {
				t.Fatalf("CGO_ENABLED present = %v, want %v (value %q)", ok, tc.wantCGOSet, got)
			}
			if tc.wantCGOSet && got != tc.wantCGO {
				t.Fatalf("CGO_ENABLED = %q, want %q", got, tc.wantCGO)
			}
		})
	}
}

func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}
