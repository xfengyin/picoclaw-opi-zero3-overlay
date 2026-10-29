package opi_zero3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeLED creates a sysfs LED directory with function and trigger attributes.
func fakeLED(t *testing.T, sysRoot, name, function, trigger string) *LED {
	t.Helper()
	dir := filepath.Join(sysRoot, "sys", "class", "leds", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brightness"), []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trigger"), []byte(trigger), 0o644); err != nil {
		t.Fatal(err)
	}
	if function != "" {
		if err := os.WriteFile(filepath.Join(dir, "function"), []byte(function), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &LED{Name: name, Dir: dir}
}

func TestDiscoverLEDByFunction(t *testing.T) {
	root := t.TempDir()
	fakeLED(t, root, "orangepi:red:power", "power", "[none]")
	fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	led, err := DiscoverLED(root)
	if err != nil {
		t.Fatal(err)
	}
	if led.Name != "orangepi:green:status" {
		t.Errorf("DiscoverLED = %q, want orangepi:green:status", led.Name)
	}
}

func TestDiscoverLEDByNameFallback(t *testing.T) {
	root := t.TempDir()
	// No `function` attributes: fall back to name matching.
	fakeLED(t, root, "orangepi:red:power", "", "[none]")
	fakeLED(t, root, "orangepi:green:status", "", "[none]")
	led, err := DiscoverLED(root)
	if err != nil {
		t.Fatal(err)
	}
	if led.Name != "orangepi:green:status" {
		t.Errorf("DiscoverLED = %q, want orangepi:green:status", led.Name)
	}
}

func TestDiscoverLEDNone(t *testing.T) {
	root := t.TempDir()
	fakeLED(t, root, "orangepi:red:power", "", "[none]")
	if _, err := DiscoverLED(root); err == nil {
		t.Fatal("expected error when no status LED exists")
	}
}

func TestLEDSetAndPrepare(t *testing.T) {
	root := t.TempDir()
	led := fakeLED(t, root, "orangepi:green:status", "status", "[mmc0] nand-disk none")
	if err := led.prepare(); err != nil {
		t.Fatal(err)
	}
	got, err := readFile(filepath.Join(led.Dir, "trigger"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "none" {
		t.Errorf("trigger after prepare = %q, want none", got)
	}
	err = led.Set(true)
	if err != nil {
		t.Fatal(err)
	}
	got, err = readFile(filepath.Join(led.Dir, "brightness"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "1" {
		t.Errorf("brightness after Set(true) = %q, want 1", got)
	}
	if err := led.Set(false); err != nil {
		t.Fatal(err)
	}
}

func TestLEDFromPath(t *testing.T) {
	root := t.TempDir()
	led := fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	l2, err := LEDFromPath(led.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if l2.Name != "orangepi:green:status" {
		t.Errorf("LEDFromPath name = %q", l2.Name)
	}
	if _, err := LEDFromPath(filepath.Join(root, "nope")); err == nil {
		t.Error("LEDFromPath on missing dir should fail")
	}
}

func newTestEngine(t *testing.T) (*PatternEngine, *LED) {
	t.Helper()
	root := t.TempDir()
	led := fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	e := NewPatternEngine(led, 10*time.Millisecond)
	// Fixed clock at Unix ms 0 so every pattern window starts at phase 0.
	e.nowFunc = func() time.Time { return time.UnixMilli(0) }
	return e, led
}

func TestPatternBrightness(t *testing.T) {
	e, _ := newTestEngine(t)
	t0 := e.nowFunc()

	expect := func(p PatternID, tm time.Time, want bool) {
		t.Helper()
		e.Set(p, 0)
		if got := e.brightness(tm); got != want {
			t.Errorf("%s at %dms = %v, want %v", p, tm.Sub(t0).Milliseconds(), got, want)
		}
	}
	expect(PatOff, t0, false)
	expect(PatActive, t0, true)

	// Idle: on for first 200ms of each 2s window.
	expect(PatIdle, t0, true)
	expect(PatIdle, t0.Add(199*time.Millisecond), true)
	expect(PatIdle, t0.Add(201*time.Millisecond), false)
	expect(PatIdle, t0.Add(1999*time.Millisecond), false)
	expect(PatIdle, t0.Add(2000*time.Millisecond), true)

	// Thinking: 120ms on / 120ms off.
	expect(PatThinking, t0.Add(50*time.Millisecond), true)
	expect(PatThinking, t0.Add(150*time.Millisecond), false)
	expect(PatThinking, t0.Add(250*time.Millisecond), true)

	// Error: 3 x (150 on / 150 off), then 750ms pause.
	expect(PatError, t0.Add(50*time.Millisecond), true)
	expect(PatError, t0.Add(200*time.Millisecond), false)
	expect(PatError, t0.Add(350*time.Millisecond), true)
	expect(PatError, t0.Add(650*time.Millisecond), true)
	expect(PatError, t0.Add(800*time.Millisecond), false)
	expect(PatError, t0.Add(950*time.Millisecond), false)

	// Thermal: 2 x (150 on / 150 off), then 700ms pause.
	expect(PatThermal, t0.Add(50*time.Millisecond), true)
	expect(PatThermal, t0.Add(350*time.Millisecond), true)
	expect(PatThermal, t0.Add(450*time.Millisecond), false)
	expect(PatThermal, t0.Add(650*time.Millisecond), false)
}

func TestPatternTransientExpiry(t *testing.T) {
	e, _ := newTestEngine(t)
	t0 := e.nowFunc()
	e.Set(PatIdle, 0)
	e.Set(PatError, 100*time.Millisecond)
	if e.Pattern() != PatError {
		t.Fatalf("pattern = %v, want error", e.Pattern())
	}
	// Still within ttl at t0+50ms.
	if !e.brightness(t0.Add(50 * time.Millisecond)) {
		t.Error("error pattern should be lit inside ttl")
	}
	// After expiry the engine reverts to the idle fallback.
	if got := e.brightness(t0.Add(150 * time.Millisecond)); got != true {
		t.Errorf("brightness after expiry = %v, want idle heartbeat on", got)
	}
	if e.Pattern() != PatIdle {
		t.Errorf("pattern after expiry = %v, want idle", e.Pattern())
	}
}

func TestPatternEngineStartStop(t *testing.T) {
	root := t.TempDir()
	led := fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	e := NewPatternEngine(led, 10*time.Millisecond)
	e.Set(PatActive, 0)
	e.Start()
	e.Start() // second Start is a no-op
	deadline := time.Now().Add(2 * time.Second)
	for {
		v, err := readFile(filepath.Join(led.Dir, "brightness"))
		if err == nil && v == "1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("engine never lit the LED")
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.Stop()
	if v, _ := readFile(filepath.Join(led.Dir, "brightness")); v != "0" {
		t.Errorf("LED after Stop = %q, want 0", v)
	}
	e.Stop() // second Stop is a no-op
}

func TestPatternString(t *testing.T) {
	for _, p := range []PatternID{PatOff, PatIdle, PatActive, PatThinking, PatError, PatThermal} {
		if strings.TrimSpace(p.String()) == "" {
			t.Errorf("PatternID %d has empty String()", int(p))
		}
	}
}

// TestDiscoverLEDSymlink mirrors the real board: /sys/class/leds/<name> is a
// symlink into /sys/devices, so a DirEntry.IsDir() filter skips every LED and
// discovery fails on hardware while passing against fake sysroot directories.
func TestDiscoverLEDSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "devices", "platform", "leds", "red:status")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "brightness"), []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "trigger"), []byte("[none]"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "sys", "class", "leds", "red:status")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	led, err := DiscoverLED(root)
	if err != nil {
		t.Fatal(err)
	}
	if led.Name != "red:status" {
		t.Errorf("DiscoverLED = %q, want red:status", led.Name)
	}
}
