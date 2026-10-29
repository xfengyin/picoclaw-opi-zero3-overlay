package opi_zero3

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const component = "opi_zero3"

// Defaults for the board notifier.
const (
	defaultThermalInterval = 60 * time.Second
	defaultThermalMaxC     = 85.0
	thinkingWindow         = 1500 * time.Millisecond
	errorTTL               = 5 * time.Second
	thermalTTL             = 2 * time.Second
	minDeltaApplyGap       = 150 * time.Millisecond
	ledTick                = 100 * time.Millisecond
)

// Options configures the board notifier.
type Options struct {
	// SysRoot is the sysfs/procfs root (default: host root).
	SysRoot string
	// LEDOn enables status LED signaling (default: enabled).
	LEDOn bool
	// LED pins an explicit LED; when nil the status LED is auto-discovered.
	LED *LED
	// ThermalInterval is the thermal sampling/logging interval; 0 disables.
	ThermalInterval time.Duration
	// ThermalMaxC is the °C threshold for LED overheat signaling.
	ThermalMaxC float64
}

// OptionsFromEnv builds options from PICOCLAW_OPI_* environment variables.
func OptionsFromEnv() Options {
	opts := Options{
		SysRoot:         SysRoot(),
		LEDOn:           true,
		ThermalInterval: defaultThermalInterval,
		ThermalMaxC:     defaultThermalMaxC,
	}
	if v := os.Getenv(EnvLED); v != "" {
		opts.LEDOn = truthy(v)
	}
	if v := os.Getenv(EnvLEDPath); strings.TrimSpace(v) != "" {
		if led, err := LEDFromPath(strings.TrimSpace(v)); err == nil {
			opts.LED = led
		} else {
			logger.WarnCF(component, "ignoring invalid "+EnvLEDPath, map[string]any{"error": err.Error()})
		}
	}
	if v := os.Getenv(EnvThermalInterval); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
			opts.ThermalInterval = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv(EnvThermalMax); v != "" {
		if c, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && c > 0 {
			opts.ThermalMaxC = c
		}
	}
	return opts
}

// ledState is the compact, event-driven state that decides the LED pattern.
type ledState struct {
	mu         sync.Mutex
	turnActive bool
	lastDelta  time.Time
	errUntil   time.Time
	thermalHot bool
}

func (s *ledState) onTurnStart(now time.Time) {
	s.mu.Lock()
	s.turnActive = true
	// Do not refresh lastDelta here: a fresh turn shows the steady "active"
	// pattern until the first LLM delta flips it to "thinking".
	s.mu.Unlock()
}

func (s *ledState) onDelta(now time.Time) {
	s.mu.Lock()
	s.lastDelta = now
	s.mu.Unlock()
}

func (s *ledState) onTurnEnd(now time.Time) {
	s.mu.Lock()
	s.turnActive = false
	s.lastDelta = now
	s.mu.Unlock()
}

func (s *ledState) onError(now time.Time) {
	s.mu.Lock()
	s.errUntil = now.Add(errorTTL)
	s.mu.Unlock()
}

func (s *ledState) onGatewayReady() {
	s.mu.Lock()
	s.turnActive = false
	s.mu.Unlock()
}

func (s *ledState) setThermal(hot bool) {
	s.mu.Lock()
	s.thermalHot = hot
	s.mu.Unlock()
}

// pattern computes the LED pattern with the highest priority and, for
// transient patterns, the remaining ttl.
func (s *ledState) pattern(now time.Time) (PatternID, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.thermalHot {
		return PatThermal, thermalTTL
	}
	if now.Before(s.errUntil) {
		return PatError, time.Until(s.errUntil)
	}
	if s.turnActive {
		if age := now.Sub(s.lastDelta); age < thinkingWindow {
			return PatThinking, thinkingWindow - age
		}
		return PatActive, 0
	}
	return PatIdle, 0
}

// Notifier subscribes to PicoClaw runtime events and drives the Zero3 status
// LED plus a thermal watchdog. Multiple event channels (one per AgentLoop)
// can be attached to the same instance.
type Notifier struct {
	opts    Options
	state   ledState
	engine  *PatternEngine
	thermal *thermalMonitor

	mu          sync.Mutex
	subs        []events.Subscription
	lastApplied time.Time
	started     bool

	ctx    context.Context
	cancel context.CancelFunc
}

// NewNotifier creates board support for the given sysfs root. It fails with
// ErrNotZero3 when the system is not an Orange Pi Zero3.
func NewNotifier(opts Options) (*Notifier, error) {
	if opts.SysRoot == "" {
		opts.SysRoot = DefaultSysRoot
	}
	if !Detect(opts.SysRoot) {
		return nil, ErrNotZero3
	}
	if opts.ThermalInterval == 0 {
		opts.ThermalInterval = defaultThermalInterval
	}
	if opts.ThermalMaxC <= 0 {
		opts.ThermalMaxC = defaultThermalMaxC
	}

	n := &Notifier{opts: opts, ctx: context.Background()}
	n.ctx, n.cancel = context.WithCancel(n.ctx)

	if opts.LEDOn {
		led := opts.LED
		if led == nil {
			var err error
			led, err = DiscoverLED(opts.SysRoot)
			if err != nil {
				logger.WarnCF(
					component,
					"status LED not found, LED signaling disabled",
					map[string]any{"error": err.Error()},
				)
			}
		}
		if led != nil {
			if err := led.prepare(); err != nil {
				logger.WarnCF(
					component,
					"LED trigger setup failed",
					map[string]any{"led": led.Name, "error": err.Error()},
				)
			}
			n.engine = NewPatternEngine(led, ledTick)
		}
	}

	n.thermal = newThermalMonitor(opts.SysRoot, opts.ThermalInterval, opts.ThermalMaxC, func(hot bool) {
		n.state.setThermal(hot)
		n.apply(time.Now())
	})

	return n, nil
}

// Subscribe attaches an event channel to the shared LED state and starts the
// board services on the first successful subscription.
func (n *Notifier) Subscribe(ch events.EventChannel) error {
	if n == nil || ch == nil {
		return nil
	}
	sub, err := ch.Subscribe(n.ctx, events.SubscribeOptions{
		Name:         "opi_zero3.board",
		Buffer:       64,
		Backpressure: events.DropOldest,
		Concurrency:  events.Locked,
	}, n.handle)
	if err != nil {
		return fmt.Errorf("opi_zero3: subscribe: %w", err)
	}
	n.mu.Lock()
	n.subs = append(n.subs, sub)
	if !n.started {
		n.started = true
		go n.thermal.loop(n.ctx)
		if n.engine != nil {
			n.engine.Start()
		}
		// Push the initial (idle) state immediately.
		n.apply(time.Now())
		logger.InfoCF(component, "Orange Pi Zero3 board support enabled", n.logFields())
	}
	n.mu.Unlock()
	return nil
}

func (n *Notifier) logFields() map[string]any {
	fields := map[string]any{
		"thermal_interval": n.opts.ThermalInterval.String(),
		"thermal_max_c":    n.opts.ThermalMaxC,
	}
	if n.engine != nil {
		fields["status_led"] = n.engine.led.Name
	}
	return fields
}

// Pattern returns the currently selected LED pattern.
func (n *Notifier) Pattern() PatternID {
	if n == nil || n.engine == nil {
		return PatOff
	}
	return n.engine.Pattern()
}

// handle maps runtime events to LED state.
func (n *Notifier) handle(_ context.Context, evt events.Event) error {
	now := time.Now()
	switch evt.Kind {
	case events.KindGatewayReady, events.KindGatewayStart:
		n.state.onGatewayReady()
		n.apply(now)
	case events.KindAgentTurnStart, events.KindAgentSubTurnSpawn:
		n.state.onTurnStart(now)
		n.apply(now)
	case events.KindAgentLLMDelta, events.KindAgentLLMResponse,
		events.KindAgentToolExecEnd, events.KindChannelMessageOutboundQueued:
		n.state.onDelta(now)
		n.applyThrottled(now)
	case events.KindAgentTurnEnd, events.KindAgentSubTurnEnd:
		n.state.onTurnEnd(now)
		n.apply(now)
	case events.KindAgentError, events.KindGatewayReloadFailed,
		events.KindChannelMessageOutboundFailed, events.KindChannelLifecycleStartFailed:
		n.state.onError(now)
		n.apply(now)
	}
	return nil
}

// apply pushes the current state-derived pattern to the engine.
func (n *Notifier) apply(now time.Time) {
	if n == nil || n.engine == nil {
		return
	}
	pattern, ttl := n.state.pattern(now)
	n.engine.Set(pattern, ttl)
}

// applyThrottled is apply() with a rate limit so streaming LLM deltas do not
// hammer the pattern engine.
func (n *Notifier) applyThrottled(now time.Time) {
	if n == nil || n.engine == nil {
		return
	}
	if now.Sub(n.lastApplied) < minDeltaApplyGap {
		return
	}
	n.lastApplied = now
	pattern, ttl := n.state.pattern(now)
	n.engine.Set(pattern, ttl)
}

// Close detaches all subscriptions and stops board services.
func (n *Notifier) Close() error {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	subs := n.subs
	n.subs = nil
	n.mu.Unlock()
	for _, s := range subs {
		_ = s.Close()
	}
	if n.engine != nil {
		n.engine.Stop()
	}
	if n.cancel != nil {
		n.cancel()
	}
	return nil
}

// thermalMonitor samples the CPU temperature and reports overheat state.
type thermalMonitor struct {
	sysRoot  string
	interval time.Duration
	maxC     float64
	hot      func(bool)
	wasHot   bool
}

func newThermalMonitor(sysRoot string, interval time.Duration, maxC float64, hot func(bool)) *thermalMonitor {
	return &thermalMonitor{sysRoot: sysRoot, interval: interval, maxC: maxC, hot: hot}
}

func (m *thermalMonitor) loop(ctx context.Context) {
	if m == nil || m.interval <= 0 {
		return
	}
	t := time.NewTicker(m.interval)
	defer t.Stop()
	m.check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.check()
		}
	}
}

func (m *thermalMonitor) check() {
	temp, err := CPUTemp(m.sysRoot)
	if err != nil {
		logger.DebugCF(component, "thermal read failed", map[string]any{"error": err.Error()})
		return
	}
	logger.InfoCF(component, "thermal", map[string]any{"cpu_temp_c": round1(temp)})
	switch {
	case temp >= m.maxC && !m.wasHot:
		m.wasHot = true
		m.hot(true)
		logger.WarnCF(component, "CPU overheat", map[string]any{"cpu_temp_c": round1(temp), "limit_c": m.maxC})
	case temp < m.maxC-5 && m.wasHot:
		m.wasHot = false
		m.hot(false)
		logger.InfoCF(component, "CPU temperature recovered", map[string]any{"cpu_temp_c": round1(temp)})
	}
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}

// Attach wires board support to a PicoClaw runtime event channel. It is safe
// to call once per AgentLoop: only the first call on a Zero3 creates the
// notifier, and later calls add their channel to the same instance.
func Attach(ch events.EventChannel) *Notifier {
	attachMu.Lock()
	defer attachMu.Unlock()
	opts := OptionsFromEnv()
	if !Detect(opts.SysRoot) {
		return nil
	}
	if global == nil {
		n, err := NewNotifier(opts)
		if err != nil {
			logger.WarnCF(component, "board support init failed", map[string]any{"error": err.Error()})
			return nil
		}
		global = n
	}
	if ch != nil {
		if err := global.Subscribe(ch); err != nil {
			logger.WarnCF(component, "board event subscription failed", map[string]any{"error": err.Error()})
		}
	}
	return global
}

var (
	attachMu sync.Mutex
	global   *Notifier
)
