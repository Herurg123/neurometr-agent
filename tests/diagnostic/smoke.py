"""A3.3 Windows CI: ordinary offline native report + immutable B3.1 catalog.
All diagnostic JSON stays in private temporary directories and is never logged
or uploaded. Only non-identifying timing, counts and validator verdicts escape.
"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[2]
REFERENCE = ROOT / "reference-web" / "contracts" / "tests" / "test_contracts.py"
WEB_COMMIT = "bb828bb09372e16cf63d04371d5f9367eee2526e"
CATALOG_VERSION = "2026.11.11.2"
CATALOG_SHA256 = "1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2"
CATALOG_SIZE = 6589

def go(arguments, *, stdin=None, env=None, expect_success=True):
    p = subprocess.run(["go", "run", *arguments], cwd=ROOT,
                       input=stdin, capture_output=True, timeout=110, env=env)
    if expect_success and p.returncode != 0:
        # Never echo stderr, hardware data, local paths or private JSON.
        raise AssertionError(f"Go invocation failed (exit {p.returncode}); stderr withheld")
    return p

def rejected(arguments, path, env):
    completed = go(arguments, env=env, expect_success=False)
    assert completed.returncode != 0, "untrusted catalog incorrectly accepted"
    assert not path.exists(), "untrusted catalog produced report file"

def assert_pin(ref):
    data = (ROOT / "bundled" / "catalog.json").read_bytes()
    lock = json.loads((ROOT / "bundled" / "catalog.lock.json").read_bytes())
    assert len(data) == CATALOG_SIZE, "bundled catalog length mismatch"
    assert hashlib.sha256(data).hexdigest() == CATALOG_SHA256, "bundled SHA-256 mismatch"
    assert lock == {
        "source_repo":"Herurg123/neurometr-web",
        "source_commit":WEB_COMMIT,
        "source_path":"catalog/versions/2026.11.11.2/catalog.json",
        "catalog_version":CATALOG_VERSION,
        "schema_version":"0.1.0",
        "size_bytes":CATALOG_SIZE,
        "sha256":CATALOG_SHA256,
    }, "catalog lock metadata mismatch"
    decoded = ref.decode_bounded(data,ref.LIMIT_CATALOG)
    ref.validate("catalog",decoded)
    assert decoded["catalog_version"] == CATALOG_VERSION
    assert decoded["schema_version"] == "0.1.0"
    assert len(decoded["models"]) == 2
    assert sum(len(m["artifacts"]) for m in decoded["models"]) == 2
    go(["./cmd/catalogcheck","-root",str(ROOT)])
    print("A3_3_CATALOG_PIN=PASS WEB_SOURCE_COMMIT=" + WEB_COMMIT)
    print("A3_3_CATALOG_BYTES=6589 MODELS=2 GGUF_ARTIFACTS=2 WEB_CATALOG_VALIDATION=PASS")

def check_private_report(ref, raw, *, is_test=False):
    assert 0 < len(raw) <= 2 * 1024 * 1024
    result = json.loads(raw)
    assert result["kind"] == "neurometr.diagnostic"
    assert result["schema_version"] == "0.1.0"
    uuid.UUID(result["run_id"])
    assert result["producer"]["platform"] == "windows-x64"
    assert result["hardware"]["os"]["family"] == "windows"
    assert result["hardware"]["os"]["arch"] == "x64"
    assert result["hardware"]["ram"]["physical_total_bytes"] is not None
    assert result["hardware"]["ram"]["physical_total_bytes"] > 0
    assert result["hardware"]["ram"]["available_bytes"] is not None
    assert result["hardware"]["ram"]["available_observed_at"].endswith("Z")
    assert result["hardware"]["cpu"]["logical_processors"] is not None
    assert result["hardware"]["cpu"]["logical_processors"] > 0
    assert result["hardware"]["npus"] == []
    assert result["cloud_checks"] == []
    assert result["model_assessments"] == []
    assert result["runtime_checks"] == []
    assert result["engines"] == [], "catalog entries cannot count as loaded engines"
    assert result["overall_status"] in ("ok","partial")
    comps = {x["component"]:x["status"] for x in result["components"]}
    assert comps["llmfit"] == "skipped"
    assert comps["cloud"] == "skipped"
    assert comps["hardware"] in ("ok","partial")
    catalog_ref = result["catalog_ref"]
    if is_test:
        assert any("TEST ONLY" in x for x in result["limitations"])
        assert catalog_ref["origin"] == "cached"
    else:
        assert catalog_ref == {
          "version":CATALOG_VERSION,"sha256":CATALOG_SHA256,
          "origin":"bundled","compatibility":"passed",
        }
        assert not any("TEST ONLY" in x for x in result["limitations"])
    obj = ref.decode_bounded(raw, ref.LIMIT_DIAGNOSTIC)
    ref.validate("diagnostic",obj)
    resp = go(["./cmd/contractvalidate","--root",str(ROOT),"--kind","diagnostic"],stdin=raw)
    assert json.loads(resp.stdout)["valid"] is True, "Go FullValidator rejected native JSON"
    forbidden = [
        br'"hostname"',br'"username"',br'"serial_number"',br'"mac_address"',
        br'"ip_address"',br'GITHUB_TOKEN',br'USERPROFILE',br'\\Users\\',
        br'private_key',br'client_secret',br'Bearer\s+[A-Za-z0-9]',
    ]
    for pattern in forbidden:
        if re.search(pattern,raw,re.IGNORECASE):
            raise AssertionError("PII category or secret marker leaked")
    return result

def clone_bundle(root, td):
    # Separate immutable contracts pin and the bundled files; do not edit
    # the checked out repo or the canonical Web snapshot.
    shutil.copytree(root / "contracts", td / "contracts")
    shutil.copytree(root / "bundled", td / "bundled")

def main():
    if sys.platform != "win32":
        raise RuntimeError("real Windows native smoke requires GitHub-hosted Windows x64")
    if not REFERENCE.is_file():
        raise RuntimeError("exact pinned Web Python oracle checkout missing")
    spec = importlib.util.spec_from_file_location("pinned_web_contract_oracle", REFERENCE)
    oracle = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(oracle)
    assert_pin(oracle)
    env = dict(os.environ)
    env.pop("NEUROMETR_A3_TEST_ONLY",None)
    env.pop("NEUROMETR_CATALOG_PATH",None)
    with tempfile.TemporaryDirectory(prefix="neurometr-a3-3-private-") as td:
        area = Path(td)
        reportPath = area / "diagnostic.json"
        cmd = ["./cmd/diagnose","-root",str(ROOT),"-output",str(reportPath)]
        run = go(cmd,env=env) # NORMAL MODE: no test-only flag, env or -catalog!
        assert reportPath.is_file(), "ordinary native report not exported"
        raw = reportPath.read_bytes()
        result = check_private_report(oracle,raw)
        print("A3_3_ORDINARY_WINDOWS=PASS")
        print("A3_3_CATALOG_REF_ORIGIN=bundled VERSION=" + CATALOG_VERSION)
        print("A3_3_CATALOG_REF_SHA256=" + CATALOG_SHA256)
        print("A3_3_GO_FULL_VALIDATOR=PASS")
        print("A3_3_INDEPENDENT_WEB_PYTHON=PASS")
        print("A3_3_NO_LLMFIT_OFFLINE=PASS")
        print("A3_3_NO_FAKE_TPS_OR_RUNTIME=PASS")
        print("A3_3_PII_CHECK=PASS")

        # Even a byte-identical copied catalog or original bundled file
        # cannot replace the approved bundle with -catalog in normal CLI.
        copied = area / "external-copy.json"
        shutil.copyfile(ROOT / "bundled" / "catalog.json",copied)
        assert hashlib.sha256(copied.read_bytes()).hexdigest() == CATALOG_SHA256
        override = area / "override.json"
        rejected(["./cmd/diagnose","-root",str(ROOT),
                  "-catalog",str(copied),"-output",str(override)],override,env)
        rejected(["./cmd/diagnose","-root",str(ROOT),
                  "-catalog",str(ROOT/"bundled"/"catalog.json"),"-output",str(override)],override,env)
        spoofed = dict(env)
        spoofed["NEUROMETR_CATALOG_PATH"] = str(copied)
        spoofed["NEUROMETR_A3_TEST_ONLY"] = "1"
        spoofed_report = area/"env-spoof-report.json"
        go(["./cmd/diagnose","-root",str(ROOT),"-output",str(spoofed_report)],env=spoofed)
        assert check_private_report(oracle,spoofed_report.read_bytes())["catalog_ref"]["origin"] == "bundled"
        print("A3_3_EXTERNAL_OVERRIDE=BLOCKED ENV_SPOOF=IGNORED")

        # Test-only CLI remains separate and explicitly marked, not an
        # alternative unmarked production report.
        testOut = area/"test-only-report.json"
        fixture = ROOT/"contracts"/"fixtures"/"catalog_valid.json"
        testEnv = dict(env)
        testEnv["NEUROMETR_A3_TEST_ONLY"]="1"
        go(["./cmd/diagnose","-root",str(ROOT),"-catalog",str(fixture),
            "-test-only-catalog","-output",str(testOut)],env=testEnv)
        check_private_report(oracle,testOut.read_bytes(),is_test=True)
        rejected(["./cmd/diagnose","-root",str(ROOT),"-catalog",str(fixture),
                  "-test-only-catalog","-output",str(area/"not-allowed.json")],
                 area/"not-allowed.json",env)
        print("A3_3_TEST_ONLY_ISOLATION=PASS")

        # Tampering is performed only on a disposable copy, not checked-in
        # catalog bytes. No false catalog_ref may escape.
        cases = {
          "missing_bundle": lambda p: (p/"bundled"/"catalog.json").unlink(),
          "corrupt_same_length": lambda p: (p/"bundled"/"catalog.json").write_bytes(
              (p/"bundled"/"catalog.json").read_bytes()[:145] +
              bytes([((p/"bundled"/"catalog.json").read_bytes()[145] ^ 1)]) +
              (p/"bundled"/"catalog.json").read_bytes()[146:]),
          "wrong_length": lambda p: (p/"bundled"/"catalog.json").write_bytes(
              (p/"bundled"/"catalog.json").read_bytes()+b" "),
          "wrong_lock_hash": lambda p: (p/"bundled"/"catalog.lock.json").write_bytes(
              (p/"bundled"/"catalog.lock.json").read_bytes().replace(
                  CATALOG_SHA256.encode(),b"0"*64,1)),
          "wrong_version": lambda p: (p/"bundled"/"catalog.lock.json").write_bytes(
              (p/"bundled"/"catalog.lock.json").read_bytes().replace(
                  CATALOG_VERSION.encode(),b"2026.11.11.3",1)),
        }
        for name,mutate in cases.items():
            privateRoot = area / ("damaged-"+name)
            privateRoot.mkdir()
            clone_bundle(ROOT,privateRoot)
            mutate(privateRoot)
            failed = area/(name+"-report.json")
            rejected(["./cmd/diagnose","-root",str(privateRoot),
                      "-output",str(failed)],failed,env)
            print(f"A3_3_BUNDLE_NEGATIVE={name} REJECTED=PASS")

        summary = run.stdout.decode("utf-8",errors="replace")
        match = re.search(r"HARDWARE_MS=(\d+)\s+TOTAL_MS=(\d+)\s+JSON_BYTES=(\d+)",summary)
        assert match, "native timing metadata missing"
        hardware_ms,total_ms,json_bytes = map(int,match.groups())
        assert hardware_ms <= 9500, "hardware collection exceeded approved bounded route"
        assert json_bytes == len(raw)
        print(f"A3_3_HARDWARE_MS={hardware_ms} TOTAL_MS={total_ms} JSON_BYTES={json_bytes} GPU_ENTRIES={len(result['hardware']['gpus'])}")
        print("A3_3_NATIVE_FIELD_COVERAGE OS_VERSION=%s CPU_MODEL=%s PHYSICAL_CORES=%s LOGICAL_PROCESSORS=%s RAM_TOTAL=%s RAM_AVAILABLE=%s GPU_DEDICATED_KNOWN=%d GPU_SHARED_CEILING_KNOWN=%d NPU_PROBED=false" % (
            str(result["hardware"]["os"]["version"] is not None).lower(),
            str(result["hardware"]["cpu"]["model"] is not None).lower(),
            str(result["hardware"]["cpu"]["physical_cores"] is not None).lower(),
            str(result["hardware"]["cpu"]["logical_processors"] is not None).lower(),
            str(result["hardware"]["ram"]["physical_total_bytes"] is not None).lower(),
            str(result["hardware"]["ram"]["available_bytes"] is not None).lower(),
            sum(g["memory"]["dedicated_total_bytes"] is not None for g in result["hardware"]["gpus"]),
            sum(g["memory"]["shared_total_bytes"] is not None for g in result["hardware"]["gpus"]),
        ))
        # All private reports are removed by TemporaryDirectory.

if __name__ == "__main__":
    main()
