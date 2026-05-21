package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/kakkoyun/af/internal/testutil"
)

func TestScripts(t *testing.T) {
	t.Parallel()

	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			binDir := filepath.Join(env.WorkDir, "bin")
			fakeBinDir := filepath.Join(env.WorkDir, "fakebin")

			testutil.BuildBinary(t, t.Context(), ".", filepath.Join(binDir, "af"))
			writeExternalFakes(t, fakeBinDir)

			env.Setenv("AF_TEST_FAKEBIN", fakeBinDir)
			env.Setenv("PATH", testutil.PrependPath(binDir, testutil.PrependPath(fakeBinDir, os.Getenv("PATH"))))
			return nil
		},
	})
}

func writeExternalFakes(t *testing.T, dir string) {
	t.Helper()

	for _, name := range []string{"git", "tmux", "ssh", "slicer", "sbx", "pi", "claude", "codex", "hunk"} {
		testutil.WriteExecutable(t, dir, name, "echo fake "+name)
	}

	// diffity fake: echoes its arguments so testscript can assert on the range.
	testutil.WriteExecutable(t, dir, "diffity", `echo fake diffity "$@"`)

	// tailscale fake: responds to --version and serve subcommands used by af control.
	testutil.WriteExecutable(t, dir, "tailscale", `case "$1" in
  --version) echo "fake tailscale 1.0.0" ;;
  serve)
    case "$2" in
      --bg) echo "Available on your tailnet:\nhttps://fake-node.fake-tailnet.ts.net/" ;;
      off)  echo "tailscale serve off" ;;
      status) echo "https://fake-node.fake-tailnet.ts.net/" ;;
      *) echo "fake tailscale serve $*" ;;
    esac
    ;;
  status) echo "fake tailscale status" ;;
  *) echo "fake tailscale $*" ;;
esac`)

	// superterm fake: responds to --version and lifecycle subcommands used by af control.
	testutil.WriteExecutable(t, dir, "superterm", `case "$1" in
  --version) echo "fake superterm 0.5.0" ;;
  up)     echo "Superterm running at http://localhost:7681" ;;
  down)   echo "superterm stopped" ;;
  status) echo "running at http://localhost:7681" ;;
  *) echo "fake superterm $*" ;;
esac`)
}
