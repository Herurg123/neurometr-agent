# A2 pinned llmfit CLI PoC (Windows x64)

**Status:** Draft PR; no merge, packaging, release or local owner-side development.
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
- Deadline, exit code, bounded stdout (2 MiB) and stderr (256 KiB), structured error kinds, no shell expansion and bounded strict JSON parse.
- Only `system`, `list`, `fit`, `recommend`, `search`, `info` and provider-filtered `fit` operations.
- Child process receives only minimal Windows environment and explicit isolated `LLMFIT_CUSTOM_MODELS`; excludes `GITHUB_TOKEN`, `GH_TOKEN`, service tokens, credentials and arbitrary environment variables.
- A2 does **not** map llmfit output into the canonical diagnostic schema and does **not** prove hardware compatibility beyond Windows CI runner.

## Custom overlay evidence protocol

Tag v1.1.20 source `llmfit-core/src/models.rs` reads `LLMFIT_CUSTOM_MODELS` as a JSON file containing an **array** of objects with fields `name`, `provider`, `parameter_count`, `min_ram_gb`, `recommended_ram_gb`, `min_vram_gb`, `quantization`, `context_length`, `use_case`; a canonical-name collision **replaces** the embedded record, otherwise the entry is appended. Source implementation evidence is **not** proof that end-user CLI fit/ranking changes.

Windows CI creates two temporary, synthetic overlays for the same model `neurometr-poc/NeurometrA2ProbeA`: a nominal 1B model and an intentionally larger 120B model, alongside `ProbeB` as a fixed comparison. The live tests inspect `search`, `info`, and provider-filtered `fit` output. They log separate `CUSTOM_LOAD`, `CUSTOM_FIT_EFFECT`, and `CUSTOM_RANKING_EFFECT` findings. If no observable difference, mark the particular claim `not_confirmed` rather than treating a passing test as positive evidence.

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
- Prior runs failed overall because real `list` exceeded the bounded raw stdout limit; `system`, `fit`, `recommend`, synthetic custom fit and ranking were confirmed. The revised live test will accept only the controlled `output_too_large` outcome for `list` or a successfully parsed bounded JSON response. Final all-green Windows CI is required.
