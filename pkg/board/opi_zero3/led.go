package opi_zero3

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

const ledComponent = "opi_zero3.led"

// LED is a sysfs LED that can be driven manually.
type LED struct {
	// Name is the sysfs directory name, e.g. "orangepi:green:status".
	Name string
	// Dir is the full sysfs directory path, e.g.
	// "/sys/class/leds/orangepi:green:status".
	Dir string
}

// DiscoverLED finds the board status LED.
//
// Resolution order:
//  1. LEDs whose `function` attribute is "status";
//  2. LEDs whose name ends with ":green:status" or ":status".
//
// On the Orange Pi Zero3 the mainline device tree exposes "green:power" and
// "red:status"; discovery picks the latter, which is the LED that actually
// carries the state patterns (verified on Armbian 6.18.x). The green power LED
// is left to the kernel.
func DiscoverLED(sysRoot string) (*LED, error) {
	dir := filepath.Join(sysRoot, ledsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("opi_zero3: list %s: %w", dir, err)
	}
	var byFunction, greenStatus, anyStatus []string
	for _, e := range entries {
		name := e.Name()
		// /sys/class/leds entries are symlinks to the device nodes, so
		// DirEntry.IsDir() is false for every one of them: stat the target.
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || !info.IsDir() {
			continue
		}
		if fn, err := readFile(filepath.Join(dir, name, "function")); err == nil && strings.EqualFold(fn, "status") {
			byFunction = append(byFunction, name)
			continue
		}
		switch {
		case strings.HasSuffix(name, ":green:status"):
			greenStatus = append(greenStatus, name)
		case strings.HasSuffix(name, ":status"):
			anyStatus = append(anyStatus, name)
		}
	}
	pick := func(list []string) string {
		if len(list) == 0 {
			return ""
		}
		sort.Strings(list)
		return list[0]
	}
	name := pick(byFunction)
	if name == "" {
		name = pick(greenStatus)
	}
	if name == "" {
		name = pick(anyStatus)
	}
	if name == "" {
		return nil, fmt.Errorf("opi_zero3: no status LED found under %s", dir)
	}
	return &LED{Name: name, Dir: filepath.Join(dir, name)}, nil
}

// prepare makes sure the LED trigger is "none" so that brightness writes are
// honored by the gpio-leds driver.
func (l *LED) prepare() error {
	cur, err := readFile(filepath.Join(l.Dir, "trigger"))
	if err != nil {
		return err
	}
	// Trigger files list available triggers; the active one is in brackets.
	if strings.HasPrefix(strings.TrimSpace(cur), "[none]") {
		return nil
	}
	return writeFile(filepath.Join(l.Dir, "trigger"), "none")
}

// Set switches the LED on or off via the sysfs brightness attribute.
func (l *LED) Set(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return writeFile(filepath.Join(l.Dir, "brightness"), v)
}

// PatternID identifies a LED light pattern.
type PatternID int

// LED patterns used to signal PicoClaw state on the Zero3 status LED.
const (
	// PatOff keeps the LED off.
	PatOff PatternID = iota
	// PatIdle is a slow heartbeat: service running, no turn active.
	PatIdle
	// PatActive is steady on: an agent turn is being processed.
	PatActive
	// PatThinking is a fast blink: LLM tokens are streaming.
	PatThinking
	// PatError is three short flashes followed by a pause.
	PatError
	// PatThermal is two short flashes followed by a pause: CPU overheating.
	PatThermal
)

func (p PatternID) String() string {
	switch p {
	case PatOff:
		return "off"
	case PatIdle:
		return "idle"
	case PatActive:
		return "active"
	case PatThinking:
		return "thinking"
	case PatError:
		return "error"
	case PatThermal:
		return "thermal"
	}
	return fmt.Sprintf("pattern(%d)", int(p))
}

// PatternEngine renders the current pattern to an LED on a fixed tick.
type PatternEngine struct {
	led *LED

	mu       sync.Mutex
	pattern  PatternID
	expires  time.Time // transient patterns revert to fallback at this instant
	fallback PatternID

	period  time.Duration
	nowFunc func() time.Time

	startedMu sync.Mutex
	started   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewPatternEngine creates an engine driving led with the given tick period.
func NewPatternEngine(led *LED, period time.Duration) *PatternEngine {
	if period <= 0 {
		period = 100 * time.Millisecond
	}
	return &PatternEngine{
		led:      led,
		pattern:  PatOff,
		fallback: PatIdle,
		period:   period,
		nowFunc:  time.Now,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start launches the render loop and applies the current pattern immediately.
// It is a no-op when already started.
func (e *PatternEngine) Start() {
	if e == nil || e.led == nil {
		return
	}
	e.startedMu.Lock()
	if e.started {
		e.startedMu.Unlock()
		return
	}
	e.stopCh = make(chan struct{})
	e.doneCh = make(chan struct{})
	e.started = true
	e.startedMu.Unlock()
	go e.loop()
}

// Stop terminates the render loop and switches the LED off. Safe to call when
// Start was never invoked, and idempotent.
func (e *PatternEngine) Stop() {
	if e == nil {
		return
	}
	e.startedMu.Lock()
	if !e.started {
		e.startedMu.Unlock()
		return
	}
	stopCh, doneCh := e.stopCh, e.doneCh
	e.startedMu.Unlock()
	close(stopCh)
	<-doneCh
	e.startedMu.Lock()
	if e.stopCh == stopCh {
		e.started = false
	}
	e.startedMu.Unlock()
	if e.led != nil {
		_ = e.led.Set(false)
	}
}

// Set switches to pattern p. A positive ttl makes the pattern transient: when
// it expires the engine reverts to the last persistent pattern (idle/active).
func (e *PatternEngine) Set(p PatternID, ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch p {
	case PatOff, PatIdle, PatActive, PatThinking:
		e.expires = time.Time{}
		if p != PatThinking {
			e.fallback = p
		}
	default:
		if ttl > 0 {
			e.expires = e.nowFunc().Add(ttl)
		} else {
			e.expires = time.Time{}
		}
	}
	e.pattern = p
}

// Pattern returns the currently selected pattern.
func (e *PatternEngine) Pattern() PatternID {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pattern
}

func (e *PatternEngine) loop() {
	defer close(e.doneCh)
	t := time.NewTicker(e.period)
	defer t.Stop()
	e.render()
	for {
		select {
		case <-e.stopCh:
			return
		case <-t.C:
			e.render()
		}
	}
}

func (e *PatternEngine) render() { e.renderAt(e.nowFunc()) }

// brightness computes the on/off state for pattern p at time t. Transient
// patterns revert to the fallback once expired.
func (e *PatternEngine) brightness(t time.Time) bool {
	e.mu.Lock()
	p := e.pattern
	if !e.expires.IsZero() && !t.Before(e.expires) {
		p = e.fallback
		e.pattern = p
		e.expires = time.Time{}
	}
	e.mu.Unlock()

	ms := t.UnixMilli()
	switch p {
	case PatOff:
		return false
	case PatActive:
		return true
	case PatThinking:
		return ms%240 < 120
	case PatError:
		// 3 x (150ms on, 150ms off), then 750ms pause.
		c := ms % 1650
		if c < 900 {
			return c%300 < 150
		}
		return false
	case PatThermal:
		// 2 x (150ms on, 150ms off), then 700ms pause.
		c := ms % 1300
		if c < 600 {
			return c%300 < 150
		}
		return false
	case PatIdle:
		// Heartbeat: 200ms on every 2s.
		return ms%2000 < 200
	}
	return false
}

// renderAt writes the computed brightness to the LED, logging write errors at
// most once per second.
var (
	lastLEDErrLog time.Time
	ledErrMu      sync.Mutex
)

func (e *PatternEngine) renderAt(t time.Time) {
	if e == nil || e.led == nil {
		return
	}
	on := e.brightness(t)
	if err := e.led.Set(on); err != nil {
		ledErrMu.Lock()
		now := time.Now()
		if now.Sub(lastLEDErrLog) >= time.Second {
			lastLEDErrLog = now
			logger.WarnCF(ledComponent, "LED write failed (check udev rule or run as root)", map[string]any{
				"led": e.led.Name, "error": err.Error(),
			})
		}
		ledErrMu.Unlock()
	}
}

// LEDFromPath builds an LED handle from an explicit sysfs directory.
func LEDFromPath(dir string) (*LED, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("opi_zero3: %s is not a directory", dir)
	}
	if _, err := readFile(filepath.Join(dir, "brightness")); err != nil {
		return nil, fmt.Errorf("opi_zero3: %s has no brightness attribute: %w", dir, err)
	}
	return &LED{Name: filepath.Base(dir), Dir: dir}, nil
}
