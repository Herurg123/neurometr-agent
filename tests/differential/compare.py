"""A3.1 differential oracle: frozen Web B1 Python validator versus Agent Go.
The Web source is checked out READ-ONLY at the exact approved commit in CI.
No Python validator is copied into Agent production code.
"""
import copy
import importlib.util
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
WEB = ROOT / "reference-web" / "contracts"
FIX = ROOT / "contracts" / "fixtures"
EXPECTED_SOURCE_SHA = "97aa0f4c1ceaef36608288e589b8abe935c5189e"


def reference():
    file = WEB / "tests" / "test_contracts.py"
    if not file.is_file():
        raise RuntimeError(f"missing exact Web oracle at {file}")
    spec = importlib.util.spec_from_file_location("pinned_web_b1_tests", str(file))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def baseline(name):
    return json.loads((FIX / name).read_text(encoding="utf-8"))


def mutate(kind, source, label, edit):
    instance = copy.deepcopy(source)
    edit(instance)
    return (label, kind, json.dumps(instance, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8"))


def inputs():
    data = [
        ("F01", "diagnostic", (FIX / "F01_diagnostic_windows_ok.json").read_bytes()),
        ("F02", "diagnostic", (FIX / "F02_diagnostic_llmfit_timeout.json").read_bytes()),
        ("F03", "diagnostic", (FIX / "F03_diagnostic_offline_site.json").read_bytes()),
        ("F04", "diagnostic", (FIX / "F04_diagnostic_unknown_igpu.json").read_bytes()),
        ("F05", "diagnostic", (FIX / "F05_diagnostic_cpu_vs_sycl.json").read_bytes()),
        ("F06", "diagnostic", (FIX / "F06_diagnostic_unsupported_schema.json").read_bytes()),
        ("F07", "catalog", (FIX / "F07_catalog_invalid_checksum.json").read_bytes()),
        ("F08", "catalog", (FIX / "F08_catalog_incompatible_override.json").read_bytes()),
        ("catalog_valid", "catalog", (FIX / "catalog_valid.json").read_bytes()),
        ("F07_manifest", "manifest", (FIX / "F07_catalog_invalid_checksum.manifest.json").read_bytes()),
        ("latest_pointer", "pointer", (FIX / "latest_valid.json").read_bytes()),
        ("F07_snapshot_bad_integrity", "snapshot", (FIX / "F07_catalog_invalid_checksum.json").read_bytes()),
    ]

    corpus = baseline("numeric-vectors.json")
    if len(corpus["vectors"]) != 23:
        raise RuntimeError("numeric corpus changed")
    for row in corpus["vectors"]:
        data.append(("NUMERIC_" + row["input"], "numeric", ('{"number":' + row["input"] + '}').encode("ascii")))

    d = baseline("F01_diagnostic_windows_ok.json")
    c = baseline("catalog_valid.json")
    m = baseline("F07_catalog_invalid_checksum.manifest.json")
    p = baseline("latest_valid.json")
    mutation_specs = [
        ("diagnostic",d,"D_missing_run_id",lambda x:x.pop("run_id")),
        ("diagnostic",d,"D_bad_uuid_format",lambda x:x.__setitem__("run_id","not-a-uuid")),
        ("diagnostic",d,"D_bad_started_at_format",lambda x:x.__setitem__("started_at","yesterday")),
        ("diagnostic",d,"D_missing_os",lambda x:x["hardware"].pop("os")),
        ("diagnostic",d,"D_wrong_cpu_cores_type",lambda x:x["hardware"]["cpu"].__setitem__("physical_cores","four")),
        ("diagnostic",d,"D_bad_component_enum",lambda x:x["components"][0].__setitem__("status","unknown")),
        ("diagnostic",d,"D_bad_hardware_arch",lambda x:x["hardware"]["os"].__setitem__("arch",42)),
        ("diagnostic",d,"D_engines_wrong_type",lambda x:x.__setitem__("engines",{})),
        ("diagnostic",d,"D_extra_root",lambda x:x.__setitem__("unexpected","x")),
        ("diagnostic",d,"D_extra_nested",lambda x:x["hardware"]["cpu"].__setitem__("unexpected","x")),
        ("diagnostic",d,"D_duplicate_engine",lambda x:x["engines"].append(copy.deepcopy(x["engines"][0]))),
        ("diagnostic",d,"D_duplicate_component",lambda x:x["components"].append(copy.deepcopy(x["components"][0]))),
        ("diagnostic",d,"D_missing_cloud_component",lambda x:x.__setitem__("components",[y for y in x["components"] if y["component"]!="cloud"])),
        ("diagnostic",d,"D_bad_evidence_kind",lambda x:x["hardware"]["cpu"]["evidence"].__setitem__("kind","working")),
        ("diagnostic",d,"D_wrong_nested_execution",lambda x:x["model_assessments"][0]["speed_estimate"]["execution"].__setitem__("device_path","gpu_offload")),
        ("diagnostic",d,"D_non_utc_time",lambda x:x.__setitem__("started_at","2026-11-11T10:00:00+00:00")),
        ("diagnostic",d,"D_earlier_end",lambda x:x.__setitem__("finished_at","2000-01-01T00:00:00Z")),
        ("diagnostic",d,"D_unsafe_integer",lambda x:x["hardware"]["ram"].__setitem__("physical_total_bytes",9007199254740992)),
        ("diagnostic",d,"D_nested_null_instead_of_zero",lambda x:x["hardware"]["cpu"].__setitem__("physical_cores",None)),
        ("diagnostic",d,"D_missing_runtime_checks",lambda x:x.pop("runtime_checks")),
        ("diagnostic",d,"D_cloud_http401_misclassified",lambda x:x["cloud_checks"][0].__setitem__("conclusion","connection_failed")),
        ("catalog",c,"C_missing_models",lambda x:x.pop("models")),
        ("catalog",c,"C_unknown_source",lambda x:x["models"][0]["field_provenance"]["parameters"][0].__setitem__("source_id","fake")),
        ("catalog",c,"C_extra_model_key",lambda x:x["models"][0].__setitem__("unknown","x")),
        ("catalog",c,"C_bad_parameters_type",lambda x:x["models"][0].__setitem__("parameters","1B")),
        ("catalog",c,"C_duplicate_source",lambda x:x["source_snapshots"].append(copy.deepcopy(x["source_snapshots"][0]))),
        ("catalog",c,"C_invalid_provenance_map",lambda x:x["models"][0].__setitem__("field_provenance",{})),
        ("catalog",c,"C_duplicate_model",lambda x:x["models"].append(copy.deepcopy(x["models"][0]))),
        ("catalog",c,"C_negative_artifact_size",lambda x:x["models"][0]["artifacts"][0].__setitem__("size_bytes",-1)),
        ("manifest",m,"M_wrong_catalog_path",lambda x:x["artifacts"][0].__setitem__("path","/data/catalog/versions/other/catalog.json")),
        ("manifest",m,"M_missing_signing_key",lambda x:x.pop("signing_key_id")),
        ("manifest",m,"M_bad_sha256",lambda x:x["artifacts"][0].__setitem__("sha256","bad")),
        ("manifest",m,"M_incompatible_diagnostic",lambda x:x.__setitem__("compatible_diagnostic_schema_versions",["99.0.0"])),
        ("manifest",m,"M_bad_published_at",lambda x:x.__setitem__("published_at","not-a-date")),
        ("pointer",p,"P_traversal",lambda x:x.__setitem__("manifest_path","../catalog.json")),
        ("pointer",p,"P_external_url",lambda x:x.__setitem__("manifest_path","https://attacker.invalid/catalog.json")),
        ("pointer",p,"P_extra_property",lambda x:x.__setitem__("surprise","x")),
        ("pointer",p,"P_unsupported_version",lambda x:x.__setitem__("schema_version","99.0.0")),
    ]
    data.extend(mutate(kind, src, name, op) for kind,src,name,op in mutation_specs)

    data.extend([
        ("RAW_duplicate_keys","numeric",b'{"a":{"b":1,"b":2}}'),
        ("RAW_invalid_UTF8","numeric",b'{"bad":"\xff"}'),
        ("RAW_trailing_JSON","numeric",b'{"a":1}{"b":2}'),
        ("RAW_unsafe_nested_integer","numeric",b'{"array":[9007199254740992]}'),
        ("RAW_unsafe_decimal_precision","numeric",b'{"x":1.0000000000000001}'),
        ("RAW_deep","numeric",b"["*65+b"0"+b"]"*65),
        ("RAW_duplicate_array_items","numeric",b'{"ok":[1,1,1]}'),
        ("RAW_null_unknown","numeric",b'{"known":null,"state":"unknown"}'),
        ("RAW_not_tested","numeric",b'{"status":"not_tested"}'),
    ])
    return data


def go_binary(payload, kind):
    command = [str(ROOT / "build-a3-check"), "--root", str(ROOT), "--kind", kind]
    if kind == "snapshot":
        command.append("--signature-verified")
    proc = subprocess.run(command, input=payload, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, timeout=25, check=True)
    return json.loads(proc.stdout)["valid"]


def python_oracle(ref, payload, kind):
    try:
        limit = ref.LIMIT_DIAGNOSTIC if kind in ("diagnostic", "numeric") else ref.LIMIT_CATALOG
        decoded = ref.decode_bounded(payload, limit)
        if kind == "numeric":
            return True
        if kind == "snapshot":
            manifest = ref.load_fixture("F07_catalog_invalid_checksum.manifest.json")
            ref.snapshot_integrity(manifest, payload, signature_verified=True)
            return True
        ref.validate(kind, decoded)
        return True
    except (ref.ContractError, ValueError, TypeError, KeyError, IndexError, OverflowError):
        return False


def main():
    ref = reference()
    cases = inputs()
    expected = baseline("expected-results.json")
    mismatch = []
    passed = 0
    for name, kind, payload in cases:
        py_value = python_oracle(ref,payload,kind)
        go_value = go_binary(payload,kind)
        if name in expected and py_value != expected[name]["valid"]:
            raise AssertionError(f"Web oracle disagrees with pinned expected-results: {name}")
        if name.startswith("NUMERIC_"):
            target = name[len("NUMERIC_"):]
            numeric_expected = next(v["accept"] for v in baseline("numeric-vectors.json")["vectors"] if v["input"]==target)
            if py_value != numeric_expected:
                raise AssertionError(f"Web oracle numeric drift: {name}")
        if py_value != go_value:
            mismatch.append((name, kind, py_value, go_value))
            print(f"DIFF_MISMATCH {name} kind={kind} Python={py_value} Go={go_value}")
        else:
            passed += 1
    print(f"A3_1_DIFFERENTIAL_TOTAL={len(cases)} MATCH={passed} MISMATCH={len(mismatch)}")
    print(f"CANONICAL_WEB_COMMIT={EXPECTED_SOURCE_SHA}")
    if mismatch:
        raise AssertionError(f"{len(mismatch)} Go/Python validation discrepancies; do not alter approved schemas")


if __name__ == "__main__":
    main()
