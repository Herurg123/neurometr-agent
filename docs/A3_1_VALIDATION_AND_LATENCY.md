# A3.1: full canonical schema validation in Go and llmfit v1.1.20 latency evidence

**Scope:** feature PR only in `Herurg123/neurometr-agent`, based on merged G2 `main` commit `f5e6b4a61fbda5764f6f831c3dfeae18ac7901af`. No Agent A3.2, Web edits, canonical schema changes, merge, release or deployment.

## Full Go validation implementation (v0.1.0)

- **Library:** [`github.com/santhosh-tekuri/jsonschema/v6@v6.0.3`](https://github.com/santhosh-tekuri/jsonschema/tree/v6.0.3), pinned via `go.mod` and `go.sum`. Upstream supports JSON Schema Draft 2020-12, `$ref`, nested constraints, type/enum/required/additionalProperties, and opt-in format assertions. License Apache-2.0 (future distribution must review/retain dependency notices). This is a **supported library**, not a custom JSON Schema engine.
- **Entry point:** `contract.NewFullValidator(root)` compiles the **exact** four locally pinned canonical schemas (`diagnostic`, `catalog`, `catalog-manifest`, `latest-pointer`) with `compiler.AssertFormat()`. It first runs existing `VerifyPin`/`VerifySchemaHeaders`, rejects non-fragment schema references and never fetches an untrusted remote schema.
- `FullValidator.ValidateBytes(kind, raw)` enforces strict UTF-8 parsing, duplicate-key rejection, portable numeric profile (including safe integers, decimal precision and non-finite overflow), `diagnostic <=2 MiB`, node/depth budgets (100,000/64); then the full Draft 2020-12 schema including `uuid`/`date-time`; then the existing **separate semantic layer** for statuses, error/component consistency, cloud prerequisites, CPU/SYCL execution consistency, catalog provenance/overrides, manifests and pointers.
- `cmd/contractvalidate` is a CI/test-only JSON stdin comparator; it does **not** create a diagnostic report or replace canonical Python. The legacy Go `contract.Validate` contains selected semantics/structural checks and must **not** be mistaken for the full validator. Real diagnostic validation must use `FullValidator.ValidateBytes`.
- Canonical contracts are unmodified. `contracts/contract.lock.json` retains Web source commit `97aa0f4c1ceaef36608288e589b8abe935c5189e` and exact SHA-256 per file.

## Differential testing: source of truth and acceptance

- GitHub Actions checks out `Herurg123/neurometr-web` **read-only at exact commit `97aa0f4c1ceaef36608288e589b8abe935c5189e`** into a temporary CI directory. It executes the actual Web `contracts/tests/test_contracts.py` validation functions (`decode_bounded`, `validate`, `snapshot_integrity`) as the oracle. Python dependencies match canonical Web workflow: `jsonschema==4.25.1`, `rfc3339-validator==0.1.4`, Python 3.12.
- `tests/differential/compare.py` compiles the Go CLI once, submits the **same raw bytes** to both validators and compares accept/reject results. Cases: F01–F08; valid catalog, manifest, pointer and F07 invalid-checksum snapshot; 23 portable numeric vectors; deterministic negative mutations covering mandatory/nested fields, types, enums, formats, additionalProperties, versions, provenance, cloud/status/execution, bad manifest and unsafe pointer paths; and strict parser attacks (duplicate keys, UTF-8, numeric, depth) plus normal `null`/`not_tested` representations.
- **Web canonical expected-results are independently cross-checked**. The comparison fails CI on any divergence; no schema or fixture is silently altered to 'make tests green'. The check compares validation **outcomes**, not equivalent error message text or complete error-path trees.
- Direct Go unit tests (`internal/contract/full_test.go`) also cover all F01–F08 outcomes, formats, schemas, security budgets, mutations, F05 comparability and F07 simulated snapshot failures. No live Ed25519 signature verification is claimed.

## Verified CI evidence

- Initial Ubuntu and Windows runners, [Actions 43075464199](https://github.com/Herurg123/neurometr-agent/actions/runs/43075464199): **both SUCCESS**, each `A3_1_DIFFERENTIAL_TOTAL=82 MATCH=82 MISMATCH=0`; each 156 `--- PASS:` log markers, 0 `--- FAIL:` markers, and `go vet` success. This run resolved and verified Go dependency sums. A final locked-dependency run must independently pass on the PR head.
- `go.mod`/`go.sum` were subsequently locked to the exact checksums printed by verified CI `go mod tidy`/`go mod verify`, and the workflow was tightened so `go mod tidy` must leave them **unchanged**. Use the final CI run ID to verify the post-lock commit.

### Parity limitations

- 82/82 is **empirical parity on the stated corpus**, not a proof of parity for every possible JSON Schema feature, float literal, timestamp edge, Unicode variant or enormous but still valid document. Deterministic negative coverage is finite.
- The vendor Go library implements schema assertions; Neurometr's semantic invariants remain a separate layer. Some source semantics are not expressible in schema alone.
- F07 `signature_verified` is a **simulated trust flag**. This task does not implement Ed25519 public-key verification, trusted catalog fetching or last-known-good fallback.

## llmfit v1.1.20 Windows latency evidence

**Source frozen:** [`AlexsJones/llmfit@bb5771394e88e352c266f4bf8bdb7a5295a998a8`](https://github.com/AlexsJones/llmfit/tree/bb5771394e88e352c266f4bf8bdb7a5295a998a8) (tag `v1.1.20`). **Measured:** official signed executable on GitHub-hosted Windows 2025 runner in [Agent A2 live CI run 43067630406](https://github.com/Herurg123/neurometr-agent/actions/runs/43067630406), 2026-11-11. No LLM weights downloaded; these are CLI timings, not measured model TPS.

| Operation (real official CLI) | Measured main command elapsed | Raw stdout | CLI result |
|---|---:|---:|---|
| `system --json` | 60.015 s | 571 bytes | exit 0 |
| `fit --json -n 5` | 50.363 s | 11,907 bytes | exit 0, 5 entries |
| `recommend --json --limit 5` | 50.332 s | 11,907 bytes | exit 0, 5 entries |
| `list --json` | 0.24 s before bounded pipe rejection | >=8,388,608 bytes | controlled `output_too_large`; upstream broken pipe exit 101; full JSON **not parsed** |

**Timing boundary:** A2 `launch` measures elapsed time from `exec.CommandContext.Run` to completion, including process startup, hardware detection, model evaluation/serialization and pipe handling; these stages are **not separately timestamped**. The preceding per-`Run()` `--version` check is **not included** in `Result.Duration`, but *is* included in the same request context timeout. Thus these elapsed times do not represent overall UI wall-clock cost exactly.

| Component | Confirmed from pinned source | Separately measured? |
|---|---|---|
| Subprocess spawn / initialization | Go `exec.CommandContext`, then llmfit Clap entry point | **No** (inside launch elapsed) |
| CPU and RAM detection | `SystemSpecs::detect()` calls `sysinfo` refresh | **No** |
| GPU enumeration / Windows system probes | `detect_all_gpus()` invokes vendor utilities and Win32 WMI via synchronous PowerShell `Get-CimInstance Win32_VideoController`; optional registry VRAM and physical memory WMI queries | **No** |
| Model catalog setup | `run_fit`/`run_recommend` initialize `ModelDatabase::new`, installed index, build/rank fits; `system` does not need model ranking | **No** |
| JSON generation | CLI formats `display_json_system_with_providers`, `display_json_fits` and recommend output | Output byte counts measured; **serialization duration not measured** |

**Source pointers (exact tag):** [`llmfit-tui/src/main.rs` lines 4327–4335](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-tui/src/main.rs#L4327-L4335) (`system`), [2072–2157](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-tui/src/main.rs#L2072-L2157) (`run_fit`), [2548–2608](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-tui/src/main.rs#L2548-L2608) (`run_recommend`); [`llmfit-core/src/hardware.rs` lines 151–184](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-core/src/hardware.rs#L151-L184) (system detection), [247–279](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-core/src/hardware.rs#L247-L279) (GPU scan), [1360–1390](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-core/src/hardware.rs#L1360-L1390) (Windows PowerShell WMI).

### Hypotheses, not measured causal attribution

1. **Likely hardware probe latency:** all three slow commands share `SystemSpecs::detect`; Windows GPU/WMI detection launches synchronous OS subprocesses without a per-probe deadline in the inspected code. Slow WMI, virtualization, driver queries or vendor command timeouts could dominate. **No isolated WMI timings were captured; causality not proven.**
2. **Potential repeated cold-start cost:** `Run()` invokes a separate `--version` subprocess before each command and does not cache results; each real CLI operation detects hardware again. A series of calls can repeatedly incur OS probe costs. **Per-stage contribution not measured.**
3. **Possible model database/fit cost:** `fit`/`recommend` also load the embedded catalog and evaluate/rank candidates. However `system` (without ranking) was at least as slow in these runs, so it is inappropriate to attribute the full delay to fit alone.
4. **JSON formatting itself seems unlikely to dominate** for the observed small 571/11,907-byte outputs, but byte count does not prove serialization time. Full `list` is a separate oversized-output issue.

### Bounded fast-diagnostic route proposal (NOT implemented in A3.1)

1. **Fast first:** in a separately authorized A3.2 step, gather independent OS/CPU/RAM and device identity using bounded Go-native collectors, returning partial data and `unknown` where needed. Never block hardware reporting on llmfit.
2. **Optional llmfit:** start only a small allowlisted command if the remaining diagnostic time budget permits; retain the 30-second default `Run()` timeout pending analysis. On timeout / nonzero code / truncated stdout, mark the relevant llmfit component `partial` or `failed`, preserve useful hardware and structured error details. Do **not** convert absent estimates into measured/zero values.
3. **Fit/recommend must not gate the initial fast report** while Windows-hosted timing exceeds the default timeout. Consider a distinct, user-visible non-benchmark extended calculation stage only after separate authorization and repeatable resource bounds. Do not silently raise global deadlines.
4. **Never require full `list --json`**; it exceeds the raw 8 MiB limit and is not necessary for the approved fast flow. Keep a bundled base catalog and explicit source/version marking.
5. **Next profiling experiment (requires separate permission):** one fresh, bounded Windows run with isolated wall-clock checkpoints around `--version`, `system`, and optional known safe hardware probe substeps, using no models or GPU benchmarking. Avoid uncontrolled repeated fit/recommend runs. If an upstream modification/wrapper would be needed to profile stages, propose it first rather than altering pinned llmfit silently.

**Acceptance gate:** A3.1 establishes the schema+semantic validator and documents the latency risk; it does not prove that Windows v1.1.20 can meet a 30-second diagnostic budget. Real user PC, GPU/SYCL, cloud connectivity and catalog signature verification remain `not_tested`.
