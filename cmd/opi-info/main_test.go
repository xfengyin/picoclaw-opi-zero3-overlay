package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/board/opi_zero3"
)

// zero3Sysfs builds a minimal fake sysfs/procfs tree that makes opi-info
// detect an Orange Pi Zero3, so the CLI contract can be exercised on x86 CI.
func zero3Sysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("proc/device-tree/model", "OrangePi Zero3\x00") // device-tree 属性以 NUL 结尾
	write("proc/sys/kernel/osrelease", "6.18.0-armbian\n")
	write("proc/meminfo", "MemTotal:        2022776 kB\nMemFree:          900000 kB\n")
	write("sys/class/leds/orangepi:green:power/function", "power")
	write("sys/class/leds/orangepi:red:status/function", "status")
	write("sys/class/leds/orangepi:red:status/brightness", "0")
	write("sys/class/leds/orangepi:red:status/trigger", "[none]")
	write("sys/class/thermal/thermal_zone0/type", "cpu-thermal")
	write("sys/class/thermal/thermal_zone0/temp", "55123")
	write("sys/devices/system/cpu/cpufreq/policy0/scaling_governor", "schedutil")
	write("sys/devices/system/cpu/cpufreq/policy0/scaling_cur_freq", "1500000")
	write("sys/devices/system/cpu/cpufreq/policy0/cpuinfo_max_freq", "1512000")
	return root
}

// captureStdout runs fn with os.Stdout redirected into a pipe and returns
// everything fn wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	s := <-out
	_ = r.Close()
	return s
}

// TestMainJSON pins the documented CLI contract of `opi-info -json -sysroot`:
// the flags must keep working and the machine-readable summary must stay on
// stdout as valid JSON.
//
// main() is invoked exactly once per process: it parses the global
// flag.CommandLine, which cannot be parsed twice for the same flag set.
func TestMainJSON(t *testing.T) {
	root := zero3Sysfs(t)
	args := os.Args
	os.Args = []string{"opi-info", "-json", "-sysroot", root}
	t.Cleanup(func() { os.Args = args })

	raw := captureStdout(t, main)

	var s struct {
		IsZero3  bool    `json:"is_zero3"`
		Model    string  `json:"model"`
		Kernel   string  `json:"kernel"`
		Governor string  `json:"governor"`
		CPUMHz   int64   `json:"cpu_mhz"`
		MaxMHz   int64   `json:"cpu_max_mhz"`
		TempC    float64 `json:"cpu_temp_c"`
		MemKB    int64   `json:"mem_kb"`
		LED      string  `json:"status_led"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("opi-info -json 输出不是合法 JSON: %v\n输出: %q", err, raw)
	}
	if !s.IsZero3 {
		t.Error("is_zero3 = false，伪 sysfs 已写入 OrangePi Zero3 的 device-tree model")
	}
	if s.Model != "OrangePi Zero3" {
		t.Errorf("model = %q，期待去掉 NUL 后的 device-tree model", s.Model)
	}
	if s.Kernel != "6.18.0-armbian" {
		t.Errorf("kernel = %q", s.Kernel)
	}
	// README/docs 声称接管的是红灯 red:status（function=status 优先于名字回退）
	if s.LED != "orangepi:red:status" {
		t.Errorf("status_led = %q，期待 function=status 的那颗灯", s.LED)
	}
	if s.Governor != "schedutil" || s.CPUMHz != 1500 || s.MaxMHz != 1512 {
		t.Errorf("cpufreq = %s %d/%d MHz，期待 schedutil 1500/1512", s.Governor, s.CPUMHz, s.MaxMHz)
	}
	if s.TempC < 55.1 || s.TempC > 55.2 {
		t.Errorf("cpu_temp_c = %v，期待 ~55.123", s.TempC)
	}
	if s.MemKB != 2022776 {
		t.Errorf("mem_kb = %d", s.MemKB)
	}
}

// TestPrintText pins the human-readable output shape, including the "-"
// fallback used when a field is unavailable.
func TestPrintText(t *testing.T) {
	root := zero3Sysfs(t)
	out := captureStdout(t, func() { printText(opi_zero3.Summarize(root)) })
	for _, want := range []string{
		"Board       : Orange Pi Zero3",
		"Model       : OrangePi Zero3",
		"Kernel      : 6.18.0-armbian",
		"CPU         : 1500 MHz (max 1512 MHz, governor schedutil)",
		"CPU temp    : 55.1 °C",
		"RAM         : 1975 MB",
		"Status LED  : orangepi:red:status",
		"Watchdog    : false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q\n实际输出:\n%s", want, out)
		}
	}

	empty := captureStdout(t, func() { printText(opi_zero3.Summary{}) })
	if !strings.Contains(empty, "Board       : (not an Orange Pi Zero3)") {
		t.Errorf("非 Zero3 时缺少提示行:\n%s", empty)
	}
	if !strings.Contains(empty, "Status LED  : -") || !strings.Contains(empty, "governor -)") {
		t.Errorf("缺失字段应回退到 \"-\":\n%s", empty)
	}
}
