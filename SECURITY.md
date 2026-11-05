# Security Policy

## Supported versions

Only the default branch (`main`) is supported. Older commits and tags are not maintained.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

This project is self-hosted on Gitea at `http://192.168.31.101:3100/admin/picoclaw-opi-zero3-overlay`.
Report privately by one of:

1. **Gitea issue with restricted visibility** — if the repository is private, open an issue and mark it clearly `security`.
2. **Direct contact** — reach the maintainer (`admin`) out of band with a short description and reproduction steps.

Please include: the affected commit, the impact, and a minimal reproduction. I will acknowledge receipt as soon as possible and coordinate a fix and disclosure.

## Scope notes

This repository is an **overlay**: it patches an upstream checkout (`sipeed/picoclaw`) at a pinned commit and never ships a full tree. Vulnerabilities in upstream code should be reported upstream first. The overlay's own surface is `apply.sh` plus the board adapters under `pkg/board/opi_zero3/` and `cmd/opi-info/`.

## Out of scope

- Issues that require physical access to the host or to the OrangePi board.
- Denial of service caused by the self-hosted CI infrastructure rather than by this repository's code.
