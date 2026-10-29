package opi_zero3

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeSysRoot builds a temp sysfs/procfs tree with an optional device-tree
// model and returns its path.
func fakeSysRoot(t *testing.T, model string) string {
	t.Helper()
	root := t.TempDir()
	if model != "" {
		dir := filepath.Join(root, "proc", "device-tree")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Device-tree properties are NUL terminated.
		if err := os.WriteFile(filepath.Join(dir, "model"), append([]byte(model), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDetect(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"OrangePi Zero3", true},
		{"Xunlong Orange Pi Zero3", true},
		{"ORANGEPI ZERO3", true},
		{"OrangePi Zero 3", true},
		{"OrangePi Zero2W", false},
		{"Raspberry Pi 4", false},
	}
	for _, c := range cases {
		root := fakeSysRoot(t, c.model)
		if got := Detect(root); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.model, got, c.want)
		}
	}
	if Detect(t.TempDir()) {
		t.Error("Detect without device-tree model should be false")
	}
}

func TestDetectForceEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvForceBoard, "1")
	if !Detect(root) {
		t.Error("force env should override detection")
	}
	t.Setenv(EnvForceBoard, "off")
	if Detect(root) {
		t.Error("force=off must not force detection")
	}
}

func TestModelAndKernel(t *testing.T) {
	root := fakeSysRoot(t, "OrangePi Zero3")
	if got := Model(root); got != "OrangePi Zero3" {
		t.Errorf("Model() = %q", got)
	}
	if KernelRelease(root) != "" {
		t.Error("KernelRelease on empty root should be empty")
	}
	if WatchdogAvailable(root) {
		t.Error("WatchdogAvailable on empty root should be false")
	}
}
