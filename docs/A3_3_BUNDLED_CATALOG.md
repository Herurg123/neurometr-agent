# A3.3: owner-approved bundled offline catalog and ordinary Windows diagnostic

## Scope and trust boundary

Agent feature branch from `Herurg123/neurometr-agent` main `4117ceadee1ac498ab22c175060848ec54b35504`. Approved canonical source is **exact** `Herurg123/neurometr-web@bb828bb09372e16cf63d04371d5f9367eee2526e`, path `catalog/versions/2026.11.11.2/catalog.json`. Web remains READ-ONLY. A3.3 does not modify wire contracts v0.1.0, user assets, servers, network checks or release process.

Bundled copy: `bundled/catalog.json` is byte-for-byte identical to the approved Web snapshot, **6589 bytes**, SHA-256 **`1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2`**. Approved `catalog_version=2026.11.11.2`, `schema_version=0.1.0`, two model entries and two GGUF artifacts. No model weights are downloaded.

Metadata lock: `bundled/catalog.lock.json`:

```json
{"source_repo":"Herurg123/neurometr-web","source_commit":"bb828bb09372e16cf63d04371d5f9367eee2526e","source_path":"catalog/versions/2026.11.11.2/catalog.json","catalog_version":"2026.11.11.2","schema_version":"0.1.0","size_bytes":6589,"sha256":"1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2"}
```

The lock is informative and checked, but **not an autonomous trust root**: `internal/catalogpin/catalog.go` independently embeds all approved constants. Lock and catalog are read exclusively from fixed `bundled/` filenames in the Agent root; neither caller-provided SHA nor metadata path is used to select the trusted catalog. Normal mode checks strict lock keys/values, regular file and no symlink, exact 6589 bytes, SHA-256, full `FullValidator.ValidateBytes("catalog", raw)` (Draft 2020-12, strict JSON, numerical profile, semantic invariants), schema version and catalog version. Failure returns `ErrCatalogRequired` and no report JSON.

**Meaning of trust:** the executable is prepared from an owner-approved Agent source tree and includes the approved immutable offline catalog. Matching bytes to that pinned approval does **not** cryptographically authenticate a remote publisher or unsigned Web manifest, and does not prove that modified Agent binaries or altered Go source are trustworthy. No Ed25519 keys, signatures, remote updates, last-known-good, fallback download or arbitrary catalog URLs are implemented. Future release/distribution integrity requires a separate design and owner authorization.

## Ordinary diagnostic export

- `go run ./cmd/diagnose -root . -output <private absolute file>` on Windows x64 now uses the pinned bundled catalog **without** setting `NEUROMETR_A3_TEST_ONLY` or passing `-catalog`. Hardware probes remain independent; no `llmfit` binary, model download, benchmark or Internet connection is needed.
- `-catalog` is rejected in ordinary mode, **even when pointing to an identical file**. `NEUROMETR_CATALOG_PATH` is ignored. The entire expected catalog identity is a build-pinned constant, not computed from arbitrary input. The test-only path still requires *both* explicit `-test-only-catalog` and `NEUROMETR_A3_TEST_ONLY=1` and accepts only the original exact pinned contract fixture by file identity; it always emits the `TEST ONLY` limitation.
- `catalog_ref` is populated only from successful verified bundle data: `origin=bundled`, `version=2026.11.11.2`, `sha256` equal to the approved content SHA-256, `compatibility=passed` **only after** checking the full catalog and v0.1.0 versions. No `TEST ONLY` limitation is present in ordinary output.
- Native Windows OS, CPU, RAM and DXGI adapter diagnostics are the same A3.2 implementation. `dedicated_available_bytes` stays unknown when not measured; shared VRAM is not added to RAM; NPU remains untested and optional llmfit `skipped` unless explicitly requested.
- `model_assessments=[]`, `runtime_checks=[]`, `cloud_checks=[]` and normally `engines=[]`: two catalog entries are **not evidence** of model execution, measured throughput or working GPU backend.
- Before returning/exporting bytes, `FullValidator.ValidateBytes("diagnostic",raw)` checks schema and semantics; strict output size is <=2 MiB. The private JSON file is never printed by the CLI. Any missing, corrupt, wrong-length, wrong-hash or incompatible bundle fails closed; no fabricated `catalog_ref`.

## Tests and reproducibility

- `go run ./cmd/catalogcheck -root .`: independently confirms the hardcoded catalog pin, lock and full schema/semantic validation. `go run ./cmd/contractcheck -root .` checks the pre-existing canonical contract pin.
- `go test -v -count=1 ./...` on Ubuntu 24.04 and Windows 2025 runs all previous A1–A3.2 regressions including F01–F08, strict input and numeric vectors, native-mock errors, PII and 2 MiB. A3.3 tests reject missing catalog, missing lock, wrong length, same-length byte corruption, wrong SHA, version, source commit and duplicate lock keys; assert ordinary success with bundled metadata, absence of invented models/TPS and rejection of `-catalog` override, even for an identical copy or original bundled path.
- Windows 2025 cloud-native smoke executes the real Windows API collectors in ordinary mode and checks `catalog_ref` values, schema and semantics through full Go and the **exact read-only Web Python validator** at `bb828bb09372e16cf63d04371d5f9367eee2526e`. It performs private missing/corrupt/length/version/hash bundle tampering in **temporary copies**, not the checked-in snapshot. Reports are deleted and never exposed via artifacts or logs. It also checks isolated test-only mode, spoofed environment variables, PII and output size.
- Only summary evidence may be logged: pass/fail, catalog identity/count, timings, JSON byte size and hardware field coverage booleans. Real runner hostname, username, GPU/CPU model names, RAM amounts and raw JSON are not logged.

## A3.3 acceptance evidence

CI run and final HEAD will be recorded in Draft PR after **all three jobs pass**. A3.3 does not authorize merge, release, deployment, user-facing ZIP, localhost UI, history, cloud checks or A3.4.

## Limitations and next recommended gate

Offline trust is limited to approved content identity in the known Agent build. There is no signed publisher manifest authentication or release signing. The trusted bundled catalog has only two Qwen entries/two GGUF artifacts; catalog presence does not mean fit, TPS, CPU/GPU benchmarking or runtime availability were measured. NPU/GPU backend execution and owner hardware remain untested. The next stage should be chosen by the orchestrator after independent Draft PR review and separate approval.
