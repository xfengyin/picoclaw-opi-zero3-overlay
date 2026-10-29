package opi_zero3

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CPUTemp reads the CPU thermal-zone temperature in °C.
//
// The H618 exposes four thermal zones (cpu/gpu/ddr/ve). We match the zone
// whose `type` contains "cpu": "cpu-thermal" on mainline kernels and
// "cpu_thermal_zone" on vendor 6.1 kernels.
func CPUTemp(sysRoot string) (float64, error) {
	dir := filepath.Join(sysRoot, thermalDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("opi_zero3: list %s: %w", dir, err)
	}
	for _, e := range entries {
		zone := filepath.Join(dir, e.Name())
		// /sys/class/thermal entries are symlinks to the device nodes, so
		// DirEntry.IsDir() is false for every one of them: stat the target.
		if info, err := os.Stat(zone); err != nil || !info.IsDir() {
			continue
		}
		typ, err := readFile(filepath.Join(zone, "type"))
		if err != nil || !strings.Contains(strings.ToLower(typ), "cpu") {
			continue
		}
		raw, err := readFile(filepath.Join(zone, "temp"))
		if err != nil {
			// /sys/class/thermal also holds cooling devices, whose type
			// ("cpufreq-cpu0", ...) matches "cpu" but which expose no temp
			// attribute. Skip them instead of failing the whole lookup.
			continue
		}
		// Kernel exposes millidegrees Celsius.
		milli, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, fmt.Errorf("opi_zero3: parse temp %q: %w", raw, err)
		}
		return milli / 1000, nil
	}
	return 0, fmt.Errorf("opi_zero3: no cpu thermal zone under %s", dir)
}

// CPUInfo describes the cpufreq policy0 state.
type CPUInfo struct {
	Governor string
	CurMHz   int64
	MinMHz   int64
	MaxMHz   int64
}

// ReadCPUInfo reads cpufreq policy0 from sysfs (best effort; empty on failure).
func ReadCPUInfo(sysRoot string) CPUInfo {
	dir := filepath.Join(sysRoot, cpuFreqDir, "policy0")
	readK := func(name string) int64 {
		s, err := readFile(filepath.Join(dir, name))
		if err != nil {
			return 0
		}
		// "scaling_cur_freq" is in kHz.
		v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return 0
		}
		return v / 1000
	}
	gov, _ := readFile(filepath.Join(dir, "scaling_governor"))
	return CPUInfo{
		Governor: gov,
		CurMHz:   readK("scaling_cur_freq"),
		MinMHz:   readK("cpuinfo_min_freq"),
		MaxMHz:   readK("cpuinfo_max_freq"),
	}
}

// MemTotalKB returns the MemTotal value from /proc/meminfo in kB.
func MemTotalKB(sysRoot string) (int64, error) {
	raw, err := readFile(filepath.Join(sysRoot, meminfoPath))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			v, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("opi_zero3: parse %q: %w", line, err)
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("opi_zero3: MemTotal not found in %s", meminfoPath)
}

// Summary is a best-effort snapshot of board state used for diagnostics and
// the startup log line.
type Summary struct {
	IsZero3  bool    `json:"is_zero3"`
	Model    string  `json:"model"`
	Kernel   string  `json:"kernel"`
	Governor string  `json:"governor"`
	CPUMHz   int64   `json:"cpu_mhz"`
	MaxMHz   int64   `json:"cpu_max_mhz"`
	TempC    float64 `json:"cpu_temp_c"`
	MemKB    int64   `json:"mem_kb"`
	LED      string  `json:"status_led"`
	Watchdog bool    `json:"watchdog"`
}

// Summarize collects a best-effort board snapshot from sysRoot. Missing
// values are left at their zero value.
func Summarize(sysRoot string) Summary {
	s := Summary{
		IsZero3:  Detect(sysRoot),
		Model:    Model(sysRoot),
		Kernel:   KernelRelease(sysRoot),
		Watchdog: WatchdogAvailable(sysRoot),
	}
	if cpu := ReadCPUInfo(sysRoot); cpu.Governor != "" {
		s.Governor = cpu.Governor
		s.CPUMHz = cpu.CurMHz
		s.MaxMHz = cpu.MaxMHz
	}
	if t, err := CPUTemp(sysRoot); err == nil {
		s.TempC = t
	}
	if m, err := MemTotalKB(sysRoot); err == nil {
		s.MemKB = m
	}
	if led, err := DiscoverLED(sysRoot); err == nil {
		s.LED = led.Name
	}
	return s
}
