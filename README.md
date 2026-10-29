# picoclaw-opi-zero3-overlay

A minimal board-adapter overlay for [sipeed/picoclaw](https://github.com/sipeed/picoclaw) targeting the **Orange Pi Zero3** (Allwinner H618).

## What this is
- The board-support layer for Orange Pi Zero3: status LED signaling, thermal watchdog, an `opi-info` diagnostic tool, config and deployment files, and docs.
- It is 21 added files plus **two one-line hook calls** on top of a specific upstream commit. It does **not** fork or re-publish the whole picoclaw codebase.

## Upstream base
| | |
|---|---|
| Upstream | `sipeed/picoclaw` (MIT) |
| Base commit | `49183d7` |
| License | MIT - see `LICENSE` (same as upstream) |

## The two hooks
`pkg/gateway/gateway.go` and `cmd/picoclaw/internal/agent/helpers.go` each gained one call: `attachBoardSupport(agentLoop)`. Everything else in this repo is additive.

## Building / testing
The board package compiles and its tests pass on an unmodified upstream base (verified in CI with `CGO_ENABLED=0` and `-tags goolm,stdjson,opi_zero3`).

```
git clone https://github.com/sipeed/picoclaw && cd picoclaw && git checkout 49183d7
# copy the files in this repo over the tree, then:
git apply hooks.patch
go build -tags goolm,stdjson,opi_zero3 ./pkg/board/... ./pkg/gateway/... ./cmd/opi-info/...
go test -tags goolm,stdjson,opi_zero3 ./pkg/board/... ./cmd/opi-info/...
```

## What to know before you use it
- This adaptation layer was developed with **substantial help from an AI coding agent**. Review the diff before relying on it.
- The private maintenance repository is not part of this overlay.
- The example config uses `<NAS-LAN-IP>` as a placeholder for your own API endpoint host.
