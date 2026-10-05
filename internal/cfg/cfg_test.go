package cfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if p := os.Getenv("CFEX_CONFIG"); p == "" || !UnderTemp(p) {
		println("refusing to run tests: CFEX_CONFIG must point under the temp dir (use `make test`)")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestUnderTemp(t *testing.T) {
	if !UnderTemp(filepath.Join(t.TempDir(), "config.yaml")) {
		t.Fatal("t.TempDir must count as temp")
	}
	home, _ := os.UserHomeDir()
	for _, bad := range []string{filepath.Join(home, ".config", "cfex", "config.yaml"), "/etc/x.yaml", filepath.Join(os.TempDir(), "..", "x", "config.yaml")} {
		if UnderTemp(bad) {
			t.Fatalf("%s must not count as temp", bad)
		}
	}
}

// Proves the fail-closed guard: pointing the config at the real location makes Save refuse and write nothing.
func TestSaveRefusesRealConfigInTestBinary(t *testing.T) {
	home, _ := os.UserHomeDir()
	real := filepath.Join(home, ".config", "cfex", "config.yaml")
	t.Setenv("CFEX_CONFIG", real)
	var before os.FileInfo
	before, _ = os.Stat(real)
	if err := Default().Save(); err == nil {
		t.Fatal("Save must refuse to write the real config from a test binary")
	}
	after, _ := os.Stat(real)
	if (before == nil) != (after == nil) || (before != nil && !before.ModTime().Equal(after.ModTime())) {
		t.Fatal("real config was touched")
	}
}
