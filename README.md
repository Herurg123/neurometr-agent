# neurometr-agent

Windows-first Go agent for Neurometr. All A2 development and testing take place in GitHub; the owner does not need a local SDK or development environment.

## A1 — approved contract pin, merged into main (G1)

- Agent PR #1 was merged at commit `6721097cd8771b62b46c7344537a2a748f600e71`. Its `contracts/` directory pins 23 byte-for-byte canonical v0.1.0 files from Web commit `97aa0f4c1ceaef36608288e589b8abe935c5189e`.
- `contracts/contract.lock.json` contains SHA-256 digests of exact file bytes, source commit and size checks; Git blob identifiers are not content SHA-256.
- GitHub Actions verifies the contract pin, F01–F08, 23 portable numeric vectors and selected semantic invariants on Windows/Ubuntu.
- Go A1 validation remains **partial**: it does not fully implement Draft 2020-12 or demonstrate complete equivalence to the canonical Python validator. Full schema and differential validation are required before acceptance of a real diagnostic JSON.
- F07 trust checks simulate an upstream trust result, not Ed25519. F08 does not prove arbitrary future `llmfit_overrides` field support.

## A2 — llmfit v1.1.20 CLI PoC (Draft PR #2)

- [A2 reproducibility, official Windows asset, commands, tests and evidence](docs/A2_LLMFIT_POC.md). Source code and cloud tests are limited to the pinned `llmfit v1.1.20` and a controlled Go child process.
- The official Windows runner has confirmed **limited** `LLMFIT_CUSTOM_MODELS` effects on synthetic model fit and ranking. The live regression test now **fails** rather than merely logging when model loading, fit changes or ranking reversal are absent.
- **Raw llmfit stdout is capped at 8 MiB**, stderr at 256 KiB. The normative final `diagnostic.json` is capped separately at **2 MiB**. An oversized full `list --json` is rejected with `output_too_large` and must not be a required step of fast diagnostics.
- `Run()` defaults to a **30-second total timeout** when not explicitly overridden; previous official Windows CI required approximately **50–60 seconds** for `system`, `fit` and `recommend` using longer test-specific deadlines. This is a **blocking performance risk for later fast-diagnostics acceptance**, not for the bounded CLI PoC. Timeouts are not globally increased.

No Agent archive, hardware diagnostics, localhost UI, remote catalog update, release or deployment is authorized by A2. PR #2 remains Draft; merge requires separate owner approval.
