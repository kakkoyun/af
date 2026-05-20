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

	for _, name := range []string{"tmux", "ssh", "slicer", "sbx", "pi", "claude", "codex"} {
		testutil.WriteExecutable(t, dir, name, "echo fake "+name)
	}
}
