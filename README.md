# picoclaw-opi-zero3-overlay

Board support for the **Orange Pi Zero3** (Allwinner H618) on top of
[sipeed/picoclaw](https://github.com/sipeed/picoclaw) — status LED, thermal watchdog,
diagnostics and deployment files, delivered as an **overlay** rather than a fork.

## What this is

- The board-support layer for Orange Pi Zero3: status-LED signalling, a thermal watchdog,
  an `opi-info` diagnostic tool, systemd units and docs.
- **21 added files plus two one-line hook calls** on top of one upstream commit.
  It does not fork or re-publish the picoclaw codebase.
- Everything is `sysfs`/`procfs` based — no cgo, no external commands — so it keeps
  working under the default `CGO_ENABLED=0` cross-compile.

## Why an overlay instead of a fork

Forking an active upstream makes every upstream change a merge, and the merge is where
board code rots. This repo instead adds a single call site in two upstream files:

```go
attachBoardSupport(agentLoop)
```

Everything else is additive, so rebasing onto a new upstream commit is "copy the files,
re-apply `hooks.patch`, rebuild". The cost is that the overlay cannot change upstream
behaviour it does not own — which is exactly the constraint that keeps it rebase-able.

## Upstream base

| | |
|---|---|
| Upstream | `sipeed/picoclaw` (MIT) |
| Base commit | `49183d7` |
| License | MIT — see `LICENSE` (same as upstream) |

## The two hooks

`pkg/gateway/gateway.go` and `cmd/picoclaw/internal/agent/helpers.go` each gained one call:
`attachBoardSupport(agentLoop)`. Everything else in this repo is additive.

## What the board layer does

### Status LED

The board layer drives the **status** LED (on the Zero3 the mainline device tree exposes
`green:power` and `red:status`; discovery picks `red:status`, the one that actually carries
state — verified on Armbian 6.18.x). The green power LED is left to the kernel.

| Pattern | Meaning |
| ------- | ------- |
| `off` | LED disabled |
| `idle` | Slow heartbeat — service running, no turn active |
| `active` | Steady on — an agent turn is being processed |
| `thinking` | Fast blink — LLM tokens are streaming |
| `error` | Three short flashes, then a pause |
| `thermal` | Two short flashes, then a pause — CPU over threshold |

Transient patterns (`active`/`thinking`/`error`) revert to the fallback pattern when they expire.

### Thermal watchdog

Reads the CPU thermal zone (`cpu-thermal` on mainline kernels, `cpu_thermal_zone` on vendor 6.1),
logs a line every 60 s by default, and switches the LED to the `thermal` pattern above **85 °C**
by default. Cooling devices that also match `cpu` in their type but expose no `temp` attribute
are skipped rather than failing the lookup.

### `opi-info`

A one-shot diagnostic binary. Sample output on a Zero3:

```text
Board       : Orange Pi Zero3
Model       : ...
Kernel      : ...
CPU         : 1512 MHz (max 1512 MHz, governor schedutil)
CPU temp    : 48.3 °C
RAM         : 1900 MB
Status LED  : orangepi:red:status
Watchdog    : true
```

It also has a `--demo` mode that cycles through every LED pattern.

## Configuration

| Variable | Default | Effect |
| -------- | ------- | ------ |
| `PICOCLAW_OPI_LED` | enabled | Set to `0`/`off`/`false` to disable the LED notifier |
| `PICOCLAW_OPI_LED_PATH` | auto-discovered | Pin an explicit sysfs LED dir, e.g. `/sys/class/leds/orangepi:red:status` |
| `PICOCLAW_OPI_THERMAL_INTERVAL` | `60` | Seconds between thermal log lines |
| `PICOCLAW_OPI_THERMAL_MAX` | `85` | °C threshold for the `thermal` LED pattern and warning |
| `PICOCLAW_OPI_SYSROOT` | `/` | Override sysfs/procfs root (tests only) |
| `PICOCLAW_OPI_FORCE_BOARD` | unset | Pretend the machine is a Zero3 (tests only) |

## Building / testing

The board package compiles and its tests pass on an unmodified upstream base (verified in CI
with `CGO_ENABLED=0` and `-tags goolm,stdjson,opi_zero3`).

```bash
git clone https://github.com/sipeed/picoclaw && cd picoclaw && git checkout 49183d7
# copy the files in this repo over the tree, then:
git apply hooks.patch
go build -tags goolm,stdjson,opi_zero3 ./pkg/board/... ./pkg/gateway/... ./cmd/opi-info/...
go test  -tags goolm,stdjson,opi_zero3 ./pkg/board/... ./cmd/opi-info/...
```

## Deployment files

`deploy/orangepi-zero3/` ships the systemd units used on the board:

- `picoclaw.service` — the agent service;
- `zramswap.service` — zram swap, useful on a 1–2 GB board.

See [`docs/orangepi-zero3.md`](docs/orangepi-zero3.md) for the full board bring-up notes.

## What to know before you use it

- This adaptation layer was developed with **substantial help from an AI coding agent**.
  Review the diff before relying on it.
- The private maintenance repository is not part of this overlay.
- The example config in the docs uses a placeholder host for your own API endpoint.
