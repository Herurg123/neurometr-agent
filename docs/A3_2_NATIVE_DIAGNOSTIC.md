# A3.2: native Windows x64 hardware-first diagnostic v0.1.0

**Status:** limited Agent Draft PR, not a release. Based on Agent `main` `21c6fa44a235fcda793791d0de373b6e23e22131` (A3.1 merged). This phase has no localhost UI, history, cloud checks, remote updates, user ZIP, model benchmarks or deployment.

## Native data sources

| Category | Native source | Populated fields | Explicitly unknown/not tested |
|---|---|---|---|
| OS | `ntdll!RtlGetVersion`, GOARCH x64 | Windows family, numeric version, arch, source evidence | No hostname, user identity, serial or installed software |
| CPU | HKLM CPU model via Windows registry; `GetLogicalProcessorInformationEx(RelationProcessorCore)`; `GetActiveProcessorCount(ALL_PROCESSOR_GROUPS)` | CPU model when safe, active physical-core and logical-processor counts | Unavailable fields are `null`, not zero; never assume SMT ratio |
| RAM | `kernel32!GlobalMemoryStatusEx` | OS-visible physical and available bytes, UTC observation timestamp, measured evidence | Does not claim physical DIMM capacity behind reserved UMA; shared GPU allocation is never added to RAM |
| GPU | DXGI 1.1 `CreateDXGIFactory1/EnumAdapters1/GetDesc1` native COM | Hardware/display-adapter name, reported dedicated capacity and shared system-memory ceiling, evidence | `dedicated_available_bytes=null`: DXGI 1.1 descriptor does **not** measure free VRAM; an empty adapter list does not establish absolute absence |
| NPU | No approved reliable native detection implemented | `npus=[]` and explicit report limitation | Detection **not tested**, not evidence of no NPU |

All mandatory probes use direct bounded Windows-native APIs with **no PowerShell/WMI in the fast route**. A probe is capped at 1.5 s by default and independent hardware collection at 8 s total; late/hung native calls may still run in a detached goroutine after deadline because Win32 synchronous APIs cannot be forcibly canceled. The report retains successfully gathered fields and returns structured generic codes without raw paths or stderr. An isolated Windows runner is a valid functional smoke, not evidence of exact timing on owner hardware.

## Canonical JSON and safety

- JSON includes actual UTC `started_at`/`finished_at`, crypto-random UUIDv4 `run_id`, `producer`, required arrays, status/error components, hardware evidence, limitations. `cloud_checks=[]`, `runtime_checks=[]`, `model_assessments=[]` unless future separately approved work implements them. No claim of model execution or measured TPS.
- When llmfit isn't requested, its component is `skipped`; no llmfit binary is needed for hardware diagnostics. When requested, only the A2 `system --json` operation runs using default **30 s** timeout; failures produce `LLMFIT_TIMEOUT`, `LLMFIT_MISSING`, `LLMFIT_INVALID_JSON`, `LLMFIT_EXIT_NONZERO` etc. Independently successful hardware remains usable and `overall_status=partial` on optional llmfit failure.
- All reports pass `contract.FullValidator.ValidateBytes("diagnostic", ...)` **before** being returned or exported, including full Draft 2020-12, semantic checks, safe numeric profile and strict **2 MiB** limit. Output uses a private temporary file, 0600 where supported, then rename. CLI prints only validation/timing counters, never hardware details or raw JSON in CI.
- Native collectors do not retrieve hostname, username, MAC/IP, serials, secrets or raw logs. Hardware name fields are filtered for potentially sensitive text even if a collector is injected. Synthetic negative tests exercise PII/path/email rejection.

## Canonical catalog_ref blocker: explicit, no fabricated production metadata

The **approved diagnostic v0.1.0 schema requires `catalog_ref.version`, `catalog_ref.sha256`, `catalog_ref.origin` and `catalog_ref.compatibility`**. No approved production catalog is shipped in Agent yet. A report generated with `catalog_ref` claiming a made-up 'bundled' production catalog would be false.

- **Ordinary export is now fail-closed for *all* catalog paths**, regardless of valid JSON Schema, byte size, SHA-256 or where a valid fixture was copied. No production trust anchor, bundled catalog or publisher-verification policy was approved, so the exporter cannot truthfully produce a production report. Passing `-catalog` in normal mode does **not** grant trust. No canonical schemas or fixtures were modified.
- For an ordinary call, `Generate` returns **`ErrCatalogRequired` before collecting hardware** and emits **no JSON**, whether catalog is missing, copied from the canonical fixture to an external folder, or is otherwise structurally valid. The standalone native hardware probes remain independently callable. Production export requires a *separately approved* catalog/trust design, not a new arbitrary caller-supplied file.
- Cloud CI **only** uses the exact SHA-pinned `contracts/fixtures/catalog_valid.json` with explicit `-test-only-catalog` and `NEUROMETR_A3_TEST_ONLY=1`, plus a mandatory `TEST ONLY` limitation in the resulting JSON. Test mode verifies the file is the **same pinned fixture** (`os.SameFile` after `Lstat`, excluding external copies/symlinks) and applies `FullValidator.ValidateBytes("catalog", raw)`. Its `catalog_ref.sha256` is the fixture digest and `compatibility=passed` means schema compatibility, **not publisher trust**. CI deletes the private report without uploading it. The test-only opt-in must not be exposed as a production trust mechanism.

## Testing

- Cross-platform Go unit matrix on Ubuntu 24.04 and Windows 2025 covers baseline F01–F08, numeric vectors, full schema/semantic validation, offline/no llmfit, GPU not reported, unknown VRAM, NPU untested, partial errors, full probe failure, timeouts, missing/invalid/nonzero llmfit via subprocess mock, privacy, unique run IDs and oversize output.
- Windows 2025 cloud-native smoke executes the real Windows API collectors and a test-only export into an ephemeral, private file. Both **Go FullValidator** and exact pinned **Web Python validator** from commit `97aa0f4c1ceaef36608288e589b8abe935c5189e` must accept it. Only verdicts, elapsed milliseconds, JSON byte count and adapter count are logged; no raw report is printed or published.
- No real model downloads, GPU/SYCL benchmarks, llmfit production binary invocation, user-PC tests, NPU probe or external network requests.

## Reproduction on authorized CI

`go test -v -count=1 ./...`; `go vet ./...`; `python tests/diagnostic/smoke.py` on the GitHub Windows job with the pinned source oracle. To reproduce the test-only private export within a disposable Windows runner, use `NEUROMETR_A3_TEST_ONLY=1` plus `go run ./cmd/diagnose -root . -catalog <absolute path to test fixture> -output <private path> -test-only-catalog`; this is **not** a user production command. A genuine signed/bundled catalog integration requires orchestrator approval.

## Acceptance limitations / next gate

1. No approved production catalog or trust anchor exists yet, so **all ordinary diagnostic exports are blocked**, including copied fixtures and arbitrary schema-valid local catalogs. **Orchestrator decision required**: approve a real bundled catalog (possibly an explicitly defined empty bootstrap snapshot, with provenance and version) or a contract evolution that can truthfully represent a missing catalog. Neither is silently implemented here.
2. Windows DXGI detection reports adapter memory ceilings, not tested GPU runtime, and can omit unsupported devices. NPU remains untested. CPU core/API accessibility varies with Windows virtualization.
3. Hardware-timeout bounds cover caller return, not forcible native API cancellation. Performance of individual API stages should be reviewed on owner hardware later.
4. Secure local GUI, report history, public binaries, cloud consent checks, Ed25519 catalog trust, real model ranking and user-machine smoke are **outside A3.2**.

## A3.2 review fixes: explicit regression matrix

**Catalog trust fix:** `TestCopiedCatalogCannotBypassProductionGate` copies the canonical synthetic fixture byte-for-byte into an external temporary directory, confirms full catalog schema validation succeeds, and verifies ordinary `Generate()` returns `ErrCatalogRequired` and zero JSON bytes. The same copied file is rejected even in test-only mode. Windows native smoke independently attempts ordinary CLI export with the external copy, requiring nonzero exit and no output file (`A3_2_COPIED_CATALOG_PRODUCTION_BYPASS=BLOCKED`). The pinned original fixture remains accepted only in explicit test-only mode.

**Status fix:** `overall_status` follows `contracts/STATUS_EXECUTION_INVARIANTS.md`: usable component + degraded component ⇒ `partial`; no usable component and an attempted failure ⇒ `failed`; usable components without degradation ⇒ `ok`.

| Hardware | llmfit | Expected overall | Full validator |
|---|---|---|---|
| failed | ok (mock CLI) | **partial** | required PASS |
| failed | skipped | **failed** | required PASS |
| failed | failed (missing CLI) | **failed** | required PASS |
| ok | failed (missing CLI) | **partial** | required PASS |

Every row asserts component statuses, error counts and canonical `FullValidator.ValidateBytes("diagnostic", ...)` acceptance. It does not test inference, publisher authenticity or llmfit hardware performance.
## Cloud acceptance evidence (confirmed, 11 November 2026)

- [GitHub Actions run **43081281784**](https://github.com/Herurg123/neurometr-agent/actions/runs/43081281784) on code HEAD `9dee0138d700cbb05f092515864f5f76681725cd`: **3/3 jobs success**, with no FAIL. Ubuntu 24.04 Go tests: **169 PASS markers**, 0 FAIL, 2 expected upstream-CLI live SKIPs. Windows 2025 x64 Go tests: **169 PASS markers**, 0 FAIL, 2 expected SKIPs. The test inventory includes canonical F01–F08 and the newly added diagnostic negative suites.
- Real hosted Windows 2025 native smoke: **`A3_2_NATIVE_WINDOWS=PASS`, `A3_2_FULL_GO_VALIDATOR=PASS`, `A3_2_INDEPENDENT_WEB_ORACLE=PASS`, `A3_2_PII_SCAN=PASS`, `A3_2_NO_CATALOG_FAIL_CLOSED=PASS`, `A3_2_NO_LLMFIT_OFFLINE=PASS`**. Private report was destroyed after test, with no uploaded or displayed raw JSON.
- **Measured on that runner:** native hardware probes **9 ms**, whole `Generate` (including local catalog fixture validation and FullValidator) **23 ms**, canonical JSON **2325 bytes**, DXGI adapter entries **1**. Earlier successful run [43080822810](https://github.com/Herurg123/neurometr-agent/actions/runs/43080822810) measured 12 ms hardware, 30 ms total, 2324 bytes, one adapter. These are measurements of ephemeral GitHub hardware and are **not guarantees for owner PCs**.
- **Grounded field coverage on final smoke:** `OS_VERSION_KNOWN=true`, `CPU_MODEL_KNOWN=true`, `CPU_PHYSICAL_CORES_KNOWN=true`, `CPU_LOGICAL_KNOWN=true`, `RAM_TOTAL_KNOWN=true`, `RAM_AVAILABLE_KNOWN=true`, `GPU_DEDICATED_CAPACITY_KNOWN=0`, `GPU_SHARED_CEILING_KNOWN=1`, `NPU_PROBED=false`. No GPU dedicated capacity, free VRAM or NPU discovery is inferred from those unknowns. Shared ceiling is not added to RAM.
- The final report was generated from **real Windows API data** but with an explicitly **synthetic CI-only catalog**. Thus it demonstrates format correctness, partial-failure resilience, no-llmfit offline operation and native speed; **a production report remains gated on an approved real catalog**. The GitHub smoke never proves catalog publisher signature trust.

**STOP A3.2:** Draft PR only. No main merge, release, deployment, user ZIP, UI, history, network/cloud probes, A3.3 or modifications to canonical contracts/Web. Remaining unresolved catalog decision belongs to the orchestrator.
