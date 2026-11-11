# neurometr-agent

Windows-first Go agent for Neurometr. Implementation has **not** started beyond A1 contract pinning and limited contract conformance tests.

## A1 (Draft PR only)

- `contracts/` contains 23 byte-for-byte canonical contract documents, JSON Schema Draft 2020-12 schemas and synthetic fixtures from `Herurg123/neurometr-web` commit `97aa0f4c1ceaef36608288e589b8abe935c5189e` (Web Draft PR #1).
- `contracts/contract.lock.json` includes exact SHA-256 of each file and the immutable source commit. Git blob SHA-1 is **not** the content SHA-256.
- `go run ./cmd/contractcheck -root .` checks byte hashes, sizes, unknown files, version, source commit and schema headers.
- `go test -count=1 ./...` tests portable numerics (23 vectors), F01–F08, SHA mutation detection and selected negative semantic invariants. GitHub Actions runs on Ubuntu and Windows x64 cloud runners. No local tooling required for the owner.

**Validation scope:** Go uses a bounded strict JSON parser and selected semantic invariants. It is **not** a complete independent JSON Schema Draft 2020-12 evaluator and is **not proven equivalent** to the canonical Python implementation. Full schema/format validation and cross-language differential testing need a separate approved step. Python validator is not shipped.

F07 validates simulated hash/size/trust-flag rejection, **not Ed25519 verification**. `not_applied` overrides only pass the data contract; actual llmfit CLI acceptance is untested.

Both PRs remain Draft until orchestrator review. No merge, binary release, hardware/cloud probes, heavy benchmark, remote updates or production work is authorized.
