# envoy-go

A from-scratch Go reimplementation of the Envoy Proxy, verified phase-by-phase against upstream Envoy via a differential test harness.

## How to work on this project

Load `BOOTSTRAP_PROMPT.md` as the **first user message** of a fresh Claude Code session with the `superpowers` plugin active. The prompt will either bootstrap the project (if this is the first session ever) or resume from the on-disk state in `docs/envoy-go/` (every subsequent session).

Do not edit code by hand outside the prompt-driven workflow. Do not use `/gsd-*` commands — they are not part of this project. See `BOOTSTRAP_PROMPT.md` §3 for the full operating doctrine.

## Project state

- Reference Envoy version: see `docs/envoy-go/ENVOY_TARGET.md`.
- Active phase: see `docs/envoy-go/STATE.md`.
- Phase roadmap: see `docs/envoy-go/ROADMAP.md`.
- Recorded decisions: see `docs/envoy-go/DECISIONS.md`.
