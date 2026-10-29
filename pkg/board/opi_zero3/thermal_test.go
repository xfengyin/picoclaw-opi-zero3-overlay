package opi_zero3

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeThermalZone(t *testing.T, sysRoot, name, typ, temp string) {
	t.Helper()
	dir := filepath.Join(sysRoot, "sys", "class", "thermal", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "type"), []byte(typ), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp"), []byte(temp), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCPUTemp(t *testing.T) {
	root := t.TempDir()
	fakeThermalZone(t, root, "thermal_zone0", "cpu-thermal", "65234")
	fakeThermalZone(t, root, "thermal_zone1", "gpu_thermal_zone", "70000")
	fakeThermalZone(t, root, "thermal_zone2", "cpu_thermal_zone", "48200")

	// First matching CPU zone wins: thermal_zone0.
	temp, err := CPUTemp(root)
	if err != nil {
		t.Fatal(err)
	}
	if temp < 65.2 || temp > 65.3 {
		t.Errorf("CPUTemp = %f, want ~65.234", temp)
	}
}

func TestCPUTempMissing(t *testing.T) {
	root := t.TempDir()
	fakeThermalZone(t, root, "thermal_zone0", "gpu_thermal_zone", "70000")
	if _, err := CPUTemp(root); err == nil {
		t.Error("expected error without a CPU zone")
	}
}

func TestCPUInfo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sys", "devices", "system", "cpu", "cpufreq", "policy0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, v string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("scaling_governor", "schedutil")
	write("scaling_cur_freq", "1500000") // kHz
	write("cpuinfo_min_freq", "480000")
	write("cpuinfo_max_freq", "1512000")

	info := ReadCPUInfo(root)
	if info.Governor != "schedutil" {
		t.Errorf("Governor = %q", info.Governor)
	}
	if info.CurMHz != 1500 || info.MaxMHz != 1512 || info.MinMHz != 480 {
		t.Errorf("freqs = cur %d max %d min %d", info.CurMHz, info.MaxMHz, info.MinMHz)
	}
}

func TestMemTotalKB(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meminfo := "MemTotal:        2022776 kB\nMemFree:          900000 kB\n"
	if err := os.WriteFile(filepath.Join(dir, "meminfo"), []byte(meminfo), 0o644); err != nil {
		t.Fatal(err)
	}
	kb, err := MemTotalKB(root)
	if err != nil {
		t.Fatal(err)
	}
	if kb != 2022776 {
		t.Errorf("MemTotalKB = %d, want 2022776", kb)
	}
}

func TestSummarize(t *testing.T) {
	root := fakeSysRoot(t, "OrangePi Zero3")
	fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	fakeThermalZone(t, root, "thermal_zone0", "cpu-thermal", "55000")

	s := Summarize(root)
	if !s.IsZero3 {
		t.Error("IsZero3 = false")
	}
	if s.LED != "orangepi:green:status" {
		t.Errorf("LED = %q", s.LED)
	}
	if s.TempC < 54.9 || s.TempC > 55.1 {
		t.Errorf("TempC = %f, want ~55", s.TempC)
	}
	if s.Model != "OrangePi Zero3" {
		t.Errorf("Model = %q", s.Model)
	}
}

// TestCPUTempSymlink mirrors the real board: /sys/class/thermal/thermal_zone*
// are symlinks into /sys/devices, so discovery must stat the link target.
func TestCPUTempSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "devices", "platform", "thermal", "thermal_zone2")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "type"), []byte("cpu-thermal"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "temp"), []byte("48581"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "sys", "class", "thermal")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linkDir, "thermal_zone2")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	temp, err := CPUTemp(root)
	if err != nil {
		t.Fatal(err)
	}
	if temp < 48.5 || temp > 48.6 {
		t.Errorf("CPUTemp = %f, want ~48.581", temp)
	}
}

// TestCPUTempSkipsCoolingDevice covers the real board layout: cooling_device0
// has a type containing "cpu" but no temp attribute, and must not abort the
// lookup of the actual cpu-thermal zone.
func TestCPUTempSkipsCoolingDevice(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sys", "class", "thermal")
	if err := os.MkdirAll(filepath.Join(dir, "cooling_device0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cooling_device0", "type"), []byte("cpufreq-cpu0"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeThermalZone(t, root, "thermal_zone0", "gpu-thermal", "48662")
	fakeThermalZone(t, root, "thermal_zone2", "cpu-thermal", "48743")

	temp, err := CPUTemp(root)
	if err != nil {
		t.Fatal(err)
	}
	if temp < 48.7 || temp > 48.8 {
		t.Errorf("CPUTemp = %f, want ~48.743", temp)
	}
}
