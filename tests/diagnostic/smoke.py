"""A3.2 Windows CI smoke: real native hardware, PRIVATE JSON, independent Web oracle.
No personal hardware report is printed, attached or uploaded.
"""
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

def go(arguments, *, stdin=None, env=None, expect_success=True):
    p = subprocess.run(["go", "run", *arguments], cwd=ROOT,
                       input=stdin, capture_output=True, timeout=110, env=env)
    if expect_success and p.returncode != 0:
        # Only generic control result; avoid accidental environment disclosure.
        raise AssertionError(f"Go command failed (exit {p.returncode}); raw stderr withheld")
    return p

def main():
    if sys.platform != "win32":
        raise RuntimeError("native Windows smoke must run on GitHub-hosted Windows x64")
    if not REFERENCE.is_file():
        raise RuntimeError("exact approved Web Python validator checkout missing")
    spec = importlib.util.spec_from_file_location("neurometr_oracle", REFERENCE)
    ref = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ref)
    env = dict(os.environ)
    env["NEUROMETR_A3_TEST_ONLY"] = "1"
    with tempfile.TemporaryDirectory(prefix="neurometr-a3-private-") as td:
        out = Path(td) / "diagnostic.json"
        catalog = ROOT / "contracts" / "fixtures" / "catalog_valid.json"
        cmd = ["./cmd/diagnose", "-root", str(ROOT), "-catalog", str(catalog),
               "-output", str(out), "-test-only-catalog"]
        run = go(cmd, env=env)
        assert out.is_file(), "native report not generated"
        raw = out.read_bytes()
        assert 0 < len(raw) <= 2 * 1024 * 1024, "diagnostic size budget"
        result = json.loads(raw)
        assert result["schema_version"] == "0.1.0"
        assert result["kind"] == "neurometr.diagnostic"
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
        assert result["hardware"]["npus"] == [], "unverified NPU must not be invented"
        assert result["cloud_checks"] == []
        assert result["model_assessments"] == []
        assert result["runtime_checks"] == []
        assert result["overall_status"] in ("ok", "partial")
        assert any("TEST ONLY:" in str(x) for x in result["limitations"])
        components = {x["component"]:x["status"] for x in result["components"]}
        assert components["llmfit"] == "skipped"
        assert components["cloud"] == "skipped"
        assert components["hardware"] in ("ok", "partial")
        # Exact Web Python semantic/schema/format validator.
        obj = ref.decode_bounded(raw, ref.LIMIT_DIAGNOSTIC)
        ref.validate("diagnostic", obj)
        # Full Go schema and semantic validator, not legacy targeted validation.
        result_go = go(["./cmd/contractvalidate","--root",str(ROOT),"--kind","diagnostic"],stdin=raw)
        assert json.loads(result_go.stdout)["valid"] is True, "Go full validation rejected report"
        # Reject PII categories, secrets and local paths without logging raw JSON.
        forbidden = [
            br'"hostname"',br'"username"',br'"serial_number"',br'"mac_address"',
            br'"ip_address"',br'GITHUB_TOKEN',br'USERPROFILE',br'\\Users\\',
            br'private_key',br'client_secret',br'Bearer\s+[A-Za-z0-9]',
        ]
        for pattern in forbidden:
            if re.search(pattern,raw,re.IGNORECASE):
                raise AssertionError("PII category or secret marker leaked into diagnostic JSON")
        # Confirm no fake output is created if the production catalog is absent.
        blocked = Path(td) / "no-catalog.json"
        missing = go(["./cmd/diagnose","-root",str(ROOT),"-output",str(blocked)],env=env,expect_success=False)
        assert missing.returncode != 0 and not blocked.exists(), "missing catalog yielded fake report"
        # Identical valid fixture bytes outside contracts/fixtures must NOT
        # become a trusted production catalog via path relocation or SHA-256.
        copied = Path(td) / "copied-catalog.json"
        shutil.copyfile(catalog, copied)
        assert copied.read_bytes() == catalog.read_bytes()
        copied_output = Path(td) / "copied-catalog-export.json"
        bypass = go(["./cmd/diagnose","-root",str(ROOT),
                     "-catalog",str(copied),"-output",str(copied_output)],
                    env=env,expect_success=False)
        assert bypass.returncode != 0 and not copied_output.exists(), (
            "ordinary export trusted schema-valid copied fixture")
        print("A3_2_COPIED_CATALOG_PRODUCTION_BYPASS=BLOCKED")

        summary = run.stdout.decode("utf-8",errors="replace").strip()
        match = re.search(r"HARDWARE_MS=(\d+)\s+TOTAL_MS=(\d+)\s+JSON_BYTES=(\d+)",summary)
        assert match, "diagnostic timing metadata absent"
        hardware_ms,total_ms,json_bytes = map(int,match.groups())
        assert hardware_ms <= 9500, "independent hardware exceeded bounded 8 second route"
        assert json_bytes == len(raw), "reported JSON length mismatch"
        print("A3_2_NATIVE_WINDOWS=PASS")
        print("A3_2_FULL_GO_VALIDATOR=PASS")
        print("A3_2_INDEPENDENT_WEB_ORACLE=PASS")
        print("A3_2_PII_SCAN=PASS")
        print("A3_2_NO_CATALOG_FAIL_CLOSED=PASS")
        print("A3_2_NO_LLMFIT_OFFLINE=PASS")
        cpu = result["hardware"]["cpu"]
        os_info = result["hardware"]["os"]
        gpus = result["hardware"]["gpus"]
        gpu_dedicated_known = sum(1 for gpu in gpus if gpu["memory"]["dedicated_total_bytes"] is not None)
        gpu_shared_known = sum(1 for gpu in gpus if gpu["memory"]["shared_total_bytes"] is not None)
        print(f"A3_2_HARDWARE_MS={hardware_ms} TOTAL_MS={total_ms} JSON_BYTES={json_bytes} GPU_ENTRIES={len(gpus)}")
        print("A3_2_FIELD_COVERAGE OS_VERSION_KNOWN=%s CPU_MODEL_KNOWN=%s CPU_PHYSICAL_CORES_KNOWN=%s CPU_LOGICAL_KNOWN=%s RAM_TOTAL_KNOWN=%s RAM_AVAILABLE_KNOWN=%s GPU_DEDICATED_CAPACITY_KNOWN=%d GPU_SHARED_CEILING_KNOWN=%d NPU_PROBED=false" % (
            str(os_info["version"] is not None).lower(),
            str(cpu["model"] is not None).lower(),
            str(cpu["physical_cores"] is not None).lower(),
            str(cpu["logical_processors"] is not None).lower(),
            str(result["hardware"]["ram"]["physical_total_bytes"] is not None).lower(),
            str(result["hardware"]["ram"]["available_bytes"] is not None).lower(),
            gpu_dedicated_known,gpu_shared_known))
        # No JSON output/log is emitted; TemporaryDirectory removes all reports.

if __name__ == "__main__":
    main()
