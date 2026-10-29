package opi_zero3

import (
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/events"
)

// newTestNotifier builds a Notifier bound to a fake sysfs root with an LED.
func newTestNotifier(t *testing.T) (*Notifier, *events.EventBus) {
	t.Helper()
	root := fakeSysRoot(t, "OrangePi Zero3")
	led := fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	fakeThermalZone(t, root, "thermal_zone0", "cpu-thermal", "45000")

	n, err := NewNotifier(Options{
		SysRoot:         root,
		LEDOn:           true,
		LED:             led,
		ThermalInterval: 0, // disabled for tests
		ThermalMaxC:     85,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.engine == nil {
		t.Fatal("engine not created")
	}
	bus := events.NewBus()
	if err := n.Subscribe(bus.Channel()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n, bus
}

func publish(t *testing.T, bus *events.EventBus, kind events.Kind) {
	t.Helper()
	bus.PublishNonBlocking(events.Event{Kind: kind})
}

func waitPattern(t *testing.T, n *Notifier, want PatternID) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if n.Pattern() == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pattern = %v, want %v", n.Pattern(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNotifierEventFlow(t *testing.T) {
	n, bus := newTestNotifier(t)

	// Fresh notifier is idle.
	waitPattern(t, n, PatIdle)

	publish(t, bus, events.KindAgentTurnStart)
	waitPattern(t, n, PatActive)

	publish(t, bus, events.KindAgentLLMDelta)
	waitPattern(t, n, PatThinking)

	publish(t, bus, events.KindAgentTurnEnd)
	waitPattern(t, n, PatIdle)

	publish(t, bus, events.KindAgentError)
	waitPattern(t, n, PatError)

	// Errors are transient: clear the ttl and verify the revert path.
	n.state.mu.Lock()
	n.state.errUntil = time.Time{}
	n.state.mu.Unlock()
	n.apply(time.Now())
	waitPattern(t, n, PatIdle)
}

func TestNotifierThermalOverride(t *testing.T) {
	n, _ := newTestNotifier(t)
	waitPattern(t, n, PatIdle)

	now := time.Now()
	n.state.setThermal(true)
	n.apply(now)
	waitPattern(t, n, PatThermal)

	n.state.setThermal(false)
	n.apply(now)
	waitPattern(t, n, PatIdle)
}

func TestNotifierMultipleChannels(t *testing.T) {
	root := fakeSysRoot(t, "OrangePi Zero3")
	led := fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	n, err := NewNotifier(Options{SysRoot: root, LEDOn: true, LED: led, ThermalInterval: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })

	b1 := events.NewBus()
	b2 := events.NewBus()
	if err := n.Subscribe(b1.Channel()); err != nil {
		t.Fatal(err)
	}
	if err := n.Subscribe(b2.Channel()); err != nil {
		t.Fatal(err)
	}
	b2.PublishNonBlocking(events.Event{Kind: events.KindAgentTurnStart})
	waitPattern(t, n, PatActive)
	b1.PublishNonBlocking(events.Event{Kind: events.KindAgentTurnEnd})
	waitPattern(t, n, PatIdle)
}

func TestNewNotifierRejectsForeignBoard(t *testing.T) {
	root := fakeSysRoot(t, "Raspberry Pi 4")
	_, err := NewNotifier(Options{SysRoot: root})
	if err != ErrNotZero3 {
		t.Fatalf("err = %v, want ErrNotZero3", err)
	}
}

func TestSubscribeNilChannel(t *testing.T) {
	n, err := NewNotifier(Options{SysRoot: fakeSysRoot(t, "OrangePi Zero3"), LEDOn: false, ThermalInterval: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Subscribe(nil); err != nil {
		t.Fatalf("Subscribe(nil) = %v, want nil", err)
	}
}

func TestAttachSingleton(t *testing.T) {
	root := fakeSysRoot(t, "OrangePi Zero3")
	fakeLED(t, root, "orangepi:green:status", "status", "[none]")
	fakeThermalZone(t, root, "thermal_zone0", "cpu-thermal", "45000")
	t.Setenv(EnvSysRoot, root)
	t.Setenv(EnvLED, "1")

	attachMu.Lock()
	global = nil
	attachMu.Unlock()

	b1 := events.NewBus()
	b2 := events.NewBus()
	n1 := Attach(b1.Channel())
	if n1 == nil {
		t.Fatal("Attach returned nil on a fake Zero3")
	}
	n2 := Attach(b2.Channel())
	if n1 != n2 {
		t.Fatal("Attach must return the shared notifier instance")
	}
	t.Cleanup(func() {
		_ = n1.Close()
		attachMu.Lock()
		global = nil
		attachMu.Unlock()
	})
}

func TestAttachForeignBoard(t *testing.T) {
	t.Setenv(EnvSysRoot, t.TempDir()) // no device-tree model
	attachMu.Lock()
	global = nil
	attachMu.Unlock()
	if n := Attach(events.NewBus().Channel()); n != nil {
		t.Fatal("Attach on foreign board should return nil")
	}
}

func TestNotifierCloseIdempotent(t *testing.T) {
	n, err := NewNotifier(Options{SysRoot: fakeSysRoot(t, "OrangePi Zero3"), LEDOn: false, ThermalInterval: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
}
