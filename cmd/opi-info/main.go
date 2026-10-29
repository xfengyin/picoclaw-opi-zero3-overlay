// Command opi-info is a small Orange Pi Zero3 diagnostics tool shipped with
// the PicoClaw board adaptation. It prints a board summary, can demo the
// status LED patterns, and can loop-print the CPU temperature.
//
// Usage:
//
//	opi-info              # human-readable board summary
//	opi-info -json        # machine-readable summary
//	opi-info -led-test    # run the LED pattern demo (idle → thinking → active → error)
//	opi-info -thermal-loop 2  # print CPU temp every 2 seconds
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sipeed/picoclaw/pkg/board/opi_zero3"
)

func main() {
	var (
		jsonOut   = flag.Bool("json", false, "emit the summary as JSON")
		ledTest   = flag.Bool("led-test", false, "run a short LED pattern demo")
		thermal   = flag.Bool("thermal-loop", false, "print CPU temperature until interrupted")
		intervalS = flag.Int("interval", 2, "thermal-loop print interval in seconds")
		sysRoot   = flag.String("sysroot", "/", "sysfs/procfs root (testing)")
	)
	flag.Parse()

	s := opi_zero3.Summarize(*sysRoot)
	if *jsonOut {
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "opi-info:", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	} else {
		printText(s)
	}

	if *ledTest {
		runLEDTest(*sysRoot)
	}
	if *thermal {
		thermalLoop(*sysRoot, *intervalS)
	}
}

func printText(s opi_zero3.Summary) {
	if !s.IsZero3 {
		fmt.Println("Board       : (not an Orange Pi Zero3)")
	} else {
		fmt.Println("Board       : Orange Pi Zero3")
	}
	if s.Model != "" {
		fmt.Printf("Model       : %s\n", s.Model)
	}
	if s.Kernel != "" {
		fmt.Printf("Kernel      : %s\n", s.Kernel)
	}
	fmt.Printf("CPU         : %d MHz (max %d MHz, governor %s)\n", s.CPUMHz, s.MaxMHz, orDash(s.Governor))
	fmt.Printf("CPU temp    : %.1f °C\n", s.TempC)
	fmt.Printf("RAM         : %d MB\n", s.MemKB/1024)
	fmt.Printf("Status LED  : %s\n", orDash(s.LED))
	fmt.Printf("Watchdog    : %v\n", s.Watchdog)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func runLEDTest(sysRoot string) {
	led, err := opi_zero3.DiscoverLED(sysRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opi-info: no status LED:", err)
		os.Exit(1)
	}
	fmt.Printf("Running LED demo on %s (Ctrl+C to stop early)...\n", led.Name)
	engine := opi_zero3.NewPatternEngine(led, 100*time.Millisecond)
	engine.Start()
	defer engine.Stop()

	demo := []struct {
		p   opi_zero3.PatternID
		dur time.Duration
	}{
		{opi_zero3.PatIdle, 1500 * time.Millisecond},
		{opi_zero3.PatThinking, 1500 * time.Millisecond},
		{opi_zero3.PatActive, 1500 * time.Millisecond},
		{opi_zero3.PatError, 1500 * time.Millisecond},
	}
	for _, step := range demo {
		fmt.Printf("  pattern %-10s (%s)\n", step.p, step.dur)
		engine.Set(step.p, 0)
		time.Sleep(step.dur)
	}
	fmt.Println("Demo complete, LED restored.")
}

func thermalLoop(sysRoot string, intervalS int) {
	if intervalS <= 0 {
		intervalS = 2
	}
	fmt.Println("Printing CPU temperature (Ctrl+C to stop):")
	for {
		temp, err := opi_zero3.CPUTemp(sysRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, "opi-info:", err)
			return
		}
		fmt.Printf("%s  %.1f °C\n", time.Now().Format("15:04:05"), temp)
		time.Sleep(time.Duration(intervalS) * time.Second)
	}
}
