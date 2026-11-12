"""License obligation negatives: NOTICE, AND, MPL source, SHA, owner docs.

Synthetic source inventory tests do not attest official llmfit.exe.
"""
import copy
import hashlib
import io
import json
from pathlib import Path
import re
import shutil
import sys
import tarfile
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).parent))
import legal_compliance as legal

ROOT = Path(__file__).resolve().parents[2]


def synthetic_source_audit():
    inv = (ROOT / "packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt").read_text(encoding="utf-8")
    lines = inv.split("BEGIN_COMPONENTS\n")[1].split("\nEND_COMPONENTS")[0].splitlines()
    report = {
        "schema": "neurometr.g4.2.llmfit-cli-target-audit.v1",
        "upstream_commit": legal.UPSTREAM_COMMIT, "target": legal.TARGET,
        "feature_scope": "default", "cargo_edges": "normal,build",
        "dev_dependencies_excluded": True,
        "official_exe_compiled_graph_attested": False,
        "official_exe_embedded_web_assets_attested": False,
        "rust_license_spdx_unresolved": [],
        "rust_packages": [], "npm_production_packages": [
            {"name": n, "version": v, "license": "MIT"} for n, v in sorted(legal.NPM)],
    }
    sections = ["BEGIN_LICENSE_TEXTS"]
    missing = []
    count = 0
    for ln in lines:
        key, expr, tail = ln.split(" | ")
        name, version = key.split("@")
        filenames = [] if tail == "notices=MISSING" else tail.removeprefix("notices=").split(",")
        report["rust_packages"].append({
            "name": name, "version": version, "license_spdx": expr, "notice_files": filenames,
        })
        if not filenames:
            missing.append(key)
            continue
        for filename in filenames:
            data = ("MIT Copyright for " + key + "/" + filename + "\n").encode()
            sections += [
                "=== " + key + " / " + filename + " ===",
                "SPDX: " + expr,
                "SOURCE: https://crates.io/crates/" + name + "/" + version,
                "ORIGINAL_FILE_SHA256: " + legal.sha(data),
                data.decode("utf-8"),
            ]
            count += 1
    sections += ["END_LICENSE_TEXTS", "MISSING_STANDARD_NOTICE_FILE_NAMES: " +
                 ", ".join(missing)]
    bundle = "\n".join(sections) + "\n"
    report["rust_crate_notice_files_absent"] = missing
    report["rust_source_package_notice_missing"] = missing
    report["rust_source_notice_file_count"] = count
    report["rust_source_license_bundle_sha256"] = legal.sha(bundle.encode())
    return report, bundle


def fake_crate(name, version, license_file=True):
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        entries = {"Cargo.toml": b'[package]\nlicense = "MPL-2.0"\n',
                   "src/lib.rs": b'pub fn run() {}\n'}
        if license_file:
            entries["LICENSE"] = (
                b"Mozilla Public License Version 2.0\n"
                b"3.2. Distribution of Executable Form\n")
        for path, data in entries.items():
            header = tarfile.TarInfo(name + "-" + version + "/" + path)
            header.size = len(data)
            header.mtime = 0
            tar.addfile(header, io.BytesIO(data))
    return buf.getvalue()


class LegalAuditTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.report, cls.bundle = synthetic_source_audit()

    def test_synthetic_inventory_full_notices_and_compounds_pass(self):
        self.assertTrue(legal.check_source_audit(ROOT, self.report, self.bundle) is None)
        self.assertGreaterEqual(self.report["rust_source_notice_file_count"], 366)
    def test_missing_license_or_notice_fails(self):
        for lost in ("LICENSE", "LICENSE-BoringSSL", "LICENSE-UNICODE"):
            r = copy.deepcopy(self.report)
            target = "ring" if lost != "LICENSE-UNICODE" else "unicode-ident"
            rec = next(p for p in r["rust_packages"] if p["name"] == target)
            rec["notice_files"].remove(lost)
            with self.assertRaises(legal.LegalError):
                legal.check_source_audit(ROOT, r, self.bundle)
    def test_damaged_original_license_sha_fails(self):
        damaged = self.bundle.replace("MIT Copyright for", "MIX Copyright for", 1)
        with self.assertRaises(legal.LegalError):
            legal.check_source_audit(ROOT, self.report, damaged)
    def test_changed_aggregate_sha_fails(self):
        r = copy.deepcopy(self.report)
        r["rust_source_license_bundle_sha256"] = "0" * 64
        with self.assertRaises(legal.LegalError):
            legal.check_source_audit(ROOT, r, self.bundle)
    def test_missing_mpl_license_and_corrupt_crate_sha_fail(self):
        raw = fake_crate("colored", "3.1.1")
        self.assertIsNone(legal.validate_source_archive(raw, "colored@3.1.1", legal.sha(raw)))
        with self.assertRaises(legal.LegalError):
            legal.validate_source_archive(raw, "colored@3.1.1", "0" * 64)
        raw_missing = fake_crate("colored", "3.1.1", license_file=False)
        with self.assertRaises(legal.LegalError):
            legal.validate_source_archive(raw_missing, "colored@3.1.1",
                                          legal.sha(raw_missing))
    def test_missing_mpl_recipient_link_or_risk_acceptance_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel in legal.DOCS:
                source = ROOT / rel
                dest = root / rel
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, dest)
            self.assertIsNone(legal.check_documents(root))
            notices = root / "packaging/THIRD_PARTY_NOTICES_RU.txt"
            notices.write_text(notices.read_text(encoding="utf-8").replace(
                "https://static.crates.io/crates/colored/colored-3.1.1.crate", "URL_REMOVED"),
                encoding="utf-8")
            with self.assertRaises(legal.LegalError):
                legal.check_documents(root)
            shutil.copy2(ROOT / "packaging/THIRD_PARTY_NOTICES_RU.txt", notices)
            (root / "packaging/G4_2_OWNER_RISK_ACCEPTANCE_RU.txt").unlink()
            with self.assertRaises(legal.LegalError):
                legal.check_documents(root)
    def test_rmcp_or_clipboard_upstream_license_tampering_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel in legal.DOCS:
                dest = root / rel
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(ROOT / rel, dest)
            for name in ("rmcp-3.5.1-upstream-LICENSE.txt",
                         "clipboard-win-BSL-1.0.txt"):
                p = root / "packaging/licenses" / name
                original = p.read_bytes()
                p.write_bytes(original + b"\n# forged")
                with self.assertRaises(legal.LegalError):
                    legal.check_documents(root)
                p.write_bytes(original)
    def test_mpl_component_missing_from_rust_graph_fails(self):
        r = copy.deepcopy(self.report)
        r["rust_packages"] = [p for p in r["rust_packages"] if p["name"] != "colored"]
        with self.assertRaises(legal.LegalError):
            legal.check_source_audit(ROOT, r, self.bundle)
    def test_false_binary_attestation_is_rejected(self):
        r = copy.deepcopy(self.report)
        r["official_exe_compiled_graph_attested"] = True
        with self.assertRaises(legal.LegalError):
            legal.check_source_audit(ROOT, r, self.bundle)


if __name__ == "__main__":
    unittest.main(verbosity=2)
