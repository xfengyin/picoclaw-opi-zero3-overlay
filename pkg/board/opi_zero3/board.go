// Package opi_zero3 implements Orange Pi Zero3 (Allwinner H618) board support
// for PicoClaw: board auto-detection, status LED signaling, thermal
// monitoring and a small system-summary API used by the `opi-info` tool and
// the deployment scripts.
//
// Everything is sysfs/procfs based and dependency-free (no cgo, no external
// commands), so the package keeps working with the default CGO_ENABLED=0
// cross-compile build.
package opi_zero3

import (
	"os"
	"path/filepath"
	"strings"
)

// Environment variables understood by the board layer.
const (
	// EnvLED disables the LED notifier when set to 0/off/false.
	EnvLED = "PICOCLAW_OPI_LED"
	// EnvLEDPath pins an explicit sysfs LED directory, e.g.
	// "/sys/class/leds/orangepi:green:status".
	EnvLEDPath = "PICOCLAW_OPI_LED_PATH"
	// EnvThermalInterval sets seconds between thermal log lines (default 60).
	EnvThermalInterval = "PICOCLAW_OPI_THERMAL_INTERVAL"
	// EnvThermalMax sets the °C threshold above which the LED signals
	// overheating and a warning is logged (default 85).
	EnvThermalMax = "PICOCLAW_OPI_THERMAL_MAX"
	// EnvSysRoot overrides the sysfs/procfs root (testing only).
	EnvSysRoot = "PICOCLAW_OPI_SYSROOT"
	// EnvForceBoard pretends the machine is a Zero3 (testing only).
	EnvForceBoard = "PICOCLAW_OPI_FORCE_BOARD"
)

// DefaultSysRoot is the host root used in production.
const DefaultSysRoot = "/"

const (
	boardModelPath = "proc/device-tree/model"
	ledsDir        = "sys/class/leds"
	thermalDir     = "sys/class/thermal"
	cpuFreqDir     = "sys/devices/system/cpu/cpufreq"
	meminfoPath    = "proc/meminfo"
	osReleasePath  = "proc/sys/kernel/osrelease"
	watchdogPath   = "dev/watchdog"
)

// ErrNotZero3 is returned by constructors when the running system is not an
// Orange Pi Zero3.
var ErrNotZero3 = notZero3Error{}

type notZero3Error struct{}

func (notZero3Error) Error() string { return "not an Orange Pi Zero3" }

// SysRoot returns the sysfs/procfs root, honoring EnvSysRoot.
func SysRoot() string {
	if v := strings.TrimSpace(os.Getenv(EnvSysRoot)); v != "" {
		return v
	}
	return DefaultSysRoot
}

// readFile reads a small sysfs/procfs file and trims surrounding whitespace
// and trailing NUL bytes (device-tree properties are NUL terminated).
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimRight(string(b), "\x00")
	return strings.TrimSpace(s), nil
}

func writeFile(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Detect reports whether sysRoot looks like an Orange Pi Zero3 by checking
// the device-tree model property ("Xunlong Orange Pi Zero3" on vendor
// kernels, "OrangePi Zero3" on mainline/Armbian).
func Detect(sysRoot string) bool {
	if truthy(os.Getenv(EnvForceBoard)) {
		return true
	}
	model, err := readFile(filepath.Join(sysRoot, boardModelPath))
	if err != nil {
		return false
	}
	model = strings.ToLower(model)
	// Vendor and mainline kernels differ in spacing ("OrangePi Zero3" vs
	// "Orange Pi Zero 3"), so compare after removing all whitespace.
	model = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, model)
	return strings.Contains(model, "orangepi") && strings.Contains(model, "zero3")
}

// Model returns the device-tree model string, if any.
func Model(sysRoot string) string {
	s, _ := readFile(filepath.Join(sysRoot, boardModelPath))
	return s
}

// KernelRelease returns the running kernel release, if any.
func KernelRelease(sysRoot string) string {
	s, _ := readFile(filepath.Join(sysRoot, osReleasePath))
	return s
}

// WatchdogAvailable reports whether /dev/watchdog exists.
func WatchdogAvailable(sysRoot string) bool {
	_, err := os.Stat(filepath.Join(sysRoot, watchdogPath))
	return err == nil
}
