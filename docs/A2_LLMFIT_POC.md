# A2 pinned llmfit CLI PoC (Windows x64)

**Status:** Draft PR #2. A1/G1 already merged into Agent main; A2 changes remain in its separate feature branch. No A2 merge, packaging, release or local owner-side development.
**Source:** official upstream [AlexsJones/llmfit v1.1.20](https://github.com/AlexsJones/llmfit/releases/tag/v1.1.20), source tag commit `bb5771394e88e352c266f4bf8bdb7a5295a998a8` (2026-11-03).

## Reproducible upstream binary pin

- Archive: `llmfit-v1.1.20-x86_64-pc-windows-msvc.zip`.
- Official URL: `https://github.com/AlexsJones/llmfit/releases/download/v1.1.20/llmfit-v1.1.20-x86_64-pc-windows-msvc.zip`.
- Archive length: **6,878,103 bytes**.
- Official release metadata SHA-256: **`1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489`**.
- Upstream license: **MIT**, copyright (c) 2026 Alex Jones; the complete upstream LICENSE must be included in a future bundled distribution. This PoC does not publish or redistribute a binary.
- Repository source references (frozen tag): [CLI docs](https://github.com/AlexsJones/llmfit/blob/v1.1.20/docs/cli.md), [custom models](https://github.com/AlexsJones/llmfit/blob/v1.1.20/docs/custom-models.md), [model database](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-core/src/models.rs), [CLI implementation](https://github.com/AlexsJones/llmfit/blob/v1.1.20/llmfit-tui/src/main.rs).

## Commands (documented upstream, exercised only when CI confirms)

```text
llmfit.exe --version
llmfit.exe --no-dashboard --json system
llmfit.exe --no-dashboard --json list
llmfit.exe --no-dashboard --json fit -n 5
llmfit.exe --no-dashboard --json recommend --limit 5
llmfit.exe --no-dashboard --json search NeurometrA2ProbeA
llmfit.exe --no-dashboard --json info neurometr-poc/NeurometrA2ProbeA
llmfit.exe --no-dashboard --json fit --show-incompatible --providers neurometr-poc -n 10
```

Command-line flags are passed directly to `exec.CommandContext` as fixed allowlisted arguments (no shell). `--no-dashboard` avoids the optional background dashboard. No `bench`, `download`, `update`, `serve`, `--share`, or unspecified network/paid operations are invoked.

## Adapter scope

- Strict absolute executable path, regular file, optional executable SHA-256 pin, `--version` check for 1.1.20 before each request.
- Deadline, exit code, bounded **raw llmfit CLI stdout (8 MiB)** and stderr (256 KiB), structured error kinds, no shell expansion and bounded strict JSON parse. The **normative final `diagnostic.json` limit is separate: 2 MiB** (`contract.DiagnosticLimit`). A2 does not create diagnostic JSON.
- Only `system`, `list`, `fit`, `recommend`, `search`, `info` and provider-filtered `fit` operations.
- Child process receives only minimal Windows environment and explicit isolated `LLMFIT_CUSTOM_MODELS`; excludes `GITHUB_TOKEN`, `GH_TOKEN`, service tokens, credentials and arbitrary environment variables.
- A2 does **not** map llmfit output into the canonical diagnostic schema and does **not** prove hardware compatibility beyond Windows CI runner.

## Custom overlay evidence protocol

Tag v1.1.20 source `llmfit-core/src/models.rs` reads `LLMFIT_CUSTOM_MODELS` as a JSON file containing an **array** of objects with fields `name`, `provider`, `parameter_count`, `min_ram_gb`, `recommended_ram_gb`, `min_vram_gb`, `quantization`, `context_length`, `use_case`; a canonical-name collision **replaces** the embedded record, otherwise the entry is appended. Source implementation evidence is **not** proof that end-user CLI fit/ranking changes.

Windows CI creates two temporary, synthetic overlays for the same model `neurometr-poc/NeurometrA2ProbeA`: a nominal 1B model and an intentionally larger 120B model, alongside `ProbeB` as a fixed comparison. The live tests inspect `search`, `info`, and provider-filtered `fit` output. They assert **real CLI results**, not just log text: both synthetic entries must be discovered; ProbeA's parameter count must match each overlay; its computed fit level must change, required memory must increase and estimated TPS must decrease; and the provider-filtered relative order of ProbeA/ProbeB must reverse. In official Windows live CI, any missing observable effect triggers `t.Fatalf` and fails the job (`CUSTOM_LOAD`, `CUSTOM_FIT_EFFECT` or `CUSTOM_RANKING_EFFECT=not_confirmed`). Mock CI with no `NEUROMETR_A2_REAL_EXE` uses a documented `SKIP`, not a false PASS. This does not prove all future Neurometr override fields.

## CI and acceptance limits

Workflow `.github/workflows/a2-llmfit-poc.yml`: mock subprocess tests on Ubuntu 24.04 and Windows 2025; official Windows x64 release archive fetched into ephemeral CI **only after** SHA-256 and size verification, ZIP bounds checks, and Authenticode signature check. The archive and binary are **not committed or published**. Logs must be reviewed for measured outcome before claiming live CLI or custom-model verification.

- No large model downloads, benchmark, GPU/SYCL execution, installation or user-PC tests.
- GitHub Windows runner is virtualized. Real user hardware/runtime/network compatibility remains `not_tested`.
- Full independent Go JSON Schema Draft 2020-12 validation is **still incomplete from A1**, mandatory before accepted real `diagnostic.json`; A2 neither changes canonical schemas nor asserts Go/Python conformance.
- `LLMFIT_CUSTOM_MODELS` is not proof of safe application of `llmfit_overrides` fields from Neurometr catalogs. Approved field mapping, source provenance and actual fit/ranking effect require additional evidence.

## CI outcome

**Verified prior real CLI evidence:** [Windows CI run 43061260905](https://github.com/Herurg123/neurometr-agent/actions/runs/43061260905), official signed v1.1.20. Verified ZIP length 6,878,103 bytes, SHA-256 `1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489`, uncompressed 38,006,373 bytes, Authenticode `Valid`, `--version` = `llmfit 1.1.20`.

- Real `system` exit 0 (~60.18 s), `fit -n 5` exit 0 (~49.71 s), `recommend --limit 5` exit 0 (~51.80 s). Runner timings are NOT owner-hardware benchmarks.
- Real `list --json` exceeds both the original 2 MiB and revised 8 MiB **raw CLI stdout** caps and triggers a controlled `output_too_large` result; upstream may print a broken-pipe error when the bounded pipe closes. Full upstream listing is not required for fast diagnostics. The live test explicitly verifies safe rejection instead of treating that oversized payload as a JSON success; **`diagnostic.json` stays <=2 MiB**.
- Real `LLMFIT_CUSTOM_MODELS`: baseline synthetic search returned 0. Two temporary synthetic overlays were recognized by `search`. For ProbeA 1B: `fit_level=Good`, `memory_required_gb=1.58`, `estimated_tps=16.8`, `score=57.7`; for 120B: `Too Tight`, `74.03`, `0.2`, `46.2`. These are estimates, not benchmarks.
- Real provider-filtered fit ranking changed `ProbeA,ProbeB` to `ProbeB,ProbeA`. **Custom fit and ranking effects confirmed** for the tested fields. Does NOT prove arbitrary canonical overrides or replacement of an embedded catalog model.
- Historical oversized `list` attempts were rejected safely. The full `list --json` payload has **not** been parsed or validated and is **not required** by fast diagnostics. The verified pre-review all-green CI was [run 43064417056](https://github.com/Herurg123/neurometr-agent/actions/runs/43064417056); a **new run of strict assertions is mandatory** before final review approval.

## Latency and diagnostic acceptance gate

- `Run()` sets **30 seconds as the default timeout** when `Request.Timeout` is omitted or outside the allowed range. This is the total per-request context deadline, including the version check and command execution. `ProbeVersion()` has its own default of 10 seconds. **No global timeout was increased in A2 review.**
- The real Windows CI invokes `system`, `list`, `fit` and `recommend` with a **test-specific 75-second deadline**; the synthetic `search`/`info` custom-overlay calls use 50 seconds, and provider-filtered `fit` uses 60 seconds. These values are **test settings**, not A2 production defaults.
- On hosted Windows 2025, prior successful live CLI operations were slow: `system` 57.94 seconds, `fit -n 5` 49.96 seconds, `recommend --limit 5` 49.88 seconds ([Actions 43064417056](https://github.com/Herurg123/neurometr-agent/actions/runs/43064417056)). The CLI did not load model weights or benchmark inference. Timings are runner-specific and not evidence of owner hardware speed.
- Under the default **30-second** `Run()` deadline, the observed 50–60-second operations would time out. **This is a blocking risk for later acceptance of fast diagnostics**, but does not invalidate the limited CLI-invocation PoC. Before an authorized diagnostics phase, measure causes and adopt a bounded degradation/fallback strategy rather than silently raising all timeouts.
- The complete upstream `list --json` exceeds the **8 MiB raw CLI stdout cap**; the current adapter correctly returns `output_too_large` and may induce an upstream broken-pipe exit. It is **not a required fast-diagnostics operation**, and no claim is made that the entire list was parsed successfully.
- Estimates from synthetic custom models must not be presented as measured throughput. No owner CPU, GPU/SYCL, real network or heavy model benchmark was tested.
