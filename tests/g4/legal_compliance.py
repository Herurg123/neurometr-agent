"""G4.2 fail-closed *source-based* license/notice compliance, never a binary SBOM.

The official llmfit.exe Rust and embedded JS composition remains NOT_VERIFIED.
This check establishes redistributed notices for the pinned source closure and
live access to MPL corresponding source, not an independent build attestation.
"""
import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import tarfile
import urllib.request

UPSTREAM_COMMIT = "bb5771394e88e352c266f4bf8bdb7a5295a998a8"
TARGET = "x86_64-pc-windows-msvc"
EXPECTED_MISSING = {
    "clipboard-win@5.4.1", "llmfit@1.1.20", "llmfit-core@1.1.20",
    "rmcp@3.5.1", "rmcp-macros@3.5.1",
}
SPDX_IDS = {"MIT", "Apache-2.0", "MPL-2.0", "BSL-1.0", "BSD-3-Clause",
            "ISC", "Zlib", "0BSD", "Unlicense", "Unicode-3.0",
            "CDLA-Permissive-2.0"}
COMPOUND = {
    "ring@0.17.14": ("LICENSE", "LICENSE-BoringSSL", "LICENSE-other-bits"),
    "matchit@0.8.4": ("LICENSE", "LICENSE.httprouter"),
    "unicode-ident@1.0.24": ("LICENSE-UNICODE", "LICENSE-MIT"),
}
MPL = {
    "colored@3.1.1": (
        "https://static.crates.io/crates/colored/colored-3.1.1.crate",
        "faf9468729b8cbcea668e36183cb69d317348c2e08e994829fb56ebfdfbaac34"),
    "option-ext@0.2.0": (
        "https://static.crates.io/crates/option-ext/option-ext-0.2.0.crate",
        "04744f49eae99ab78e0d5c0b603ab218f515ea8cfe5a456d7629ad883a3b6e7d"),
}
SUPPLEMENTS = {
    "clipboard-win@5.4.1": "packaging/licenses/clipboard-win-BSL-1.0.txt",
    "llmfit@1.1.20": "packaging/licenses/llmfit-MIT.txt",
    "llmfit-core@1.1.20": "packaging/licenses/llmfit-MIT.txt",
    "rmcp@3.5.1": "packaging/licenses/rmcp-3.5.1-upstream-LICENSE.txt",
    "rmcp-macros@3.5.1": "packaging/licenses/rmcp-3.5.1-upstream-LICENSE.txt",
}
NPM = {("react", "18.3.1"), ("react-dom", "18.3.1"),
       ("scheduler", "0.23.2"), ("js-tokens", "4.0.0"),
       ("loose-envify", "1.4.0")}
DOCS = {
    "packaging/THIRD_PARTY_NOTICES_RU.txt": ("MPL-2.0", "rmcp", "Unicode-3.0"),
    "packaging/DISTRIBUTION_TERMS_RU.txt": ("НЕ отменяет", "open-source"),
    "packaging/G4_2_OWNER_RISK_ACCEPTANCE_RU.txt":
        ("BINARY_PROVENANCE = NOT_VERIFIED", "OWNER_RISK_ACCEPTANCE = APPROVED"),
    "packaging/licenses/rmcp-3.5.1-upstream-LICENSE.txt":
        ("licensing transition", "END OF TERMS AND CONDITIONS", "MIT License",
         "Copyright (c) 2024-2025"),
    "packaging/licenses/clipboard-win-BSL-1.0.txt":
        ("Boost Software License - Version 1.0", "Permission is hereby granted"),
    "packaging/licenses/llmfit-MIT.txt": ("MIT License", "Copyright (c) 2026 Alex Jones"),
    "packaging/licenses/react-family-MIT.txt": ("MIT License", "Copyright"),
    "packaging/licenses/js-tokens-MIT.txt": ("MIT", "Copyright"),
    "packaging/licenses/loose-envify-MIT.txt": ("MIT", "Copyright"),
    "packaging/licenses/jsonschema-Apache-2.0.txt":
        ("END OF TERMS AND CONDITIONS", "APPENDIX: How to apply"),
    "packaging/licenses/golang-x-text-BSD-3-Clause.txt":
        ("Redistributions", "Copyright"),
    "packaging/licenses/go-toolchain-BSD-3-Clause.txt":
        ("Redistributions", "Copyright"),
}
# Git object IDs from the exact licensed upstream source revisions.
SUPPLEMENT_BLOBS = {
    "packaging/licenses/clipboard-win-BSL-1.0.txt":
        "36b7cd93cdfbac762f5be4c6ce276df2ea6305c2",
    "packaging/licenses/rmcp-3.5.1-upstream-LICENSE.txt":
        "4a93985763241755401a10678395303de4e720ba",
}


class LegalError(ValueError):
    pass


def require(condition, why):
    if not condition:
        raise LegalError(why)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def checked_file(root, relative, max_size=2 * 1024 * 1024):
    require(not relative.startswith("/") and ".." not in Path(relative).parts,
            "illegal evidence path: " + relative)
    path = root / relative
    require(path.is_file() and not path.is_symlink(), "missing/unsafe: " + relative)
    data = path.read_bytes()
    require(0 < len(data) <= max_size, "empty/oversized: " + relative)
    return data


def check_documents(root):
    for rel, phrases in DOCS.items():
        data = checked_file(root, rel)
        txt = data.decode("utf-8")
        for phrase in phrases:
            require(phrase in txt, rel + ": missing license/notice " + phrase)
    for rel, expected in SUPPLEMENT_BLOBS.items():
        raw = checked_file(root, rel)
        gitblob = b"blob " + str(len(raw)).encode() + b"\0" + raw
        require(hashlib.sha1(gitblob).hexdigest() == expected,
                rel + ": upstream source blob mismatch")
    notices = checked_file(root, "packaging/THIRD_PARTY_NOTICES_RU.txt").decode("utf-8")
    for name, (url, expected) in MPL.items():
        require(name.replace("@", " ") in notices and url in notices and expected in notices,
                "missing complete MPL recipient source notice: " + name)
    for name in ("react 18.3.1", "react-dom 18.3.1", "scheduler 0.23.2",
                 "js-tokens 4.0.0", "loose-envify 1.4.0"):
        require(name in notices, "missing npm copyright/notice: " + name)
    require("не ограничиваются" in notices and "MIT" in notices
            and "Apache-2.0" in notices, "OSS rights or MIT/Apache transition omitted")


def validate_source_archive(raw, key, checksum):
    """Verify corresponding source bytes, copyright license and Rust source paths."""
    require(0 < len(raw) <= 2 * 1024 * 1024, "unsafe MPL source size: " + key)
    require(sha(raw) == checksum, "MPL source SHA-256 mismatch: " + key)
    name, version = key.split("@")
    prefix = name + "-" + version + "/"
    try:
        with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as archive:
            names = archive.getnames()
            require(len(names) == len(set(names)) and len(names) < 200,
                    "MPL source duplicate/unbounded entries: " + key)
            require(all(x.startswith(prefix) and ".." not in Path(x).parts
                        and not x.startswith("/") for x in names),
                    "MPL source unsafe member: " + key)
            licenses = [x for x in names
                        if x in (prefix + "LICENSE", prefix + "LICENSE.txt")]
            require(len(licenses) == 1, "MPL LICENSE missing: " + key)
            member = archive.extractfile(licenses[0])
            require(member is not None, "MPL license is not a file: " + key)
            license_text = member.read(100_000).decode("utf-8")
            require("Mozilla Public License Version 2.0" in license_text and
                    "3.2. Distribution of Executable Form" in license_text,
                    "MPL license truncated/wrong: " + key)
            require(prefix + "Cargo.toml" in names and
                    any(x.startswith(prefix + "src/") and x.endswith(".rs") for x in names),
                    "MPL corresponding source missing: " + key)
    except (tarfile.TarError, UnicodeError, OSError) as exc:
        raise LegalError("unreadable MPL source archive: " + key) from exc


def fetch_mpl_sources():
    """Actual HTTPS, bounded exact-version source verification, not only a link."""
    for key, (url, checksum) in MPL.items():
        req = urllib.request.Request(url, headers={"User-Agent": "Neurometr-G4.2-License-Audit"})
        with urllib.request.urlopen(req, timeout=35) as response:
            require(response.geturl() == url and response.status == 200,
                    "unexpected MPL source URL/HTTP: " + key)
            raw = response.read(2 * 1024 * 1024 + 1)
        validate_source_archive(raw, key, checksum)
        print("G4_2_MPL_SOURCE=PASS " + key + " SHA256=" + checksum)


def check_source_audit(root, report, license_bundle):
    require(report.get("schema") == "neurometr.g4.2.llmfit-cli-target-audit.v1"
            and report.get("upstream_commit") == UPSTREAM_COMMIT
            and report.get("target") == TARGET
            and report.get("feature_scope") == "default"
            and report.get("cargo_edges") == "normal,build"
            and report.get("dev_dependencies_excluded") is True,
            "wrong pinned CLI source audit scope")
    # Never convert reconstructed source evidence into a signed-exe assertion.
    require(report.get("official_exe_compiled_graph_attested") is False
            and report.get("official_exe_embedded_web_assets_attested") is False,
            "unverified EXE provenance falsely attested")
    records = report.get("rust_packages")
    require(isinstance(records, list) and len(records) == 215,
            "wrong Rust source closure size")
    items = {p.get("name", "") + "@" + p.get("version", ""):p for p in records}
    require(len(items) == 215 and not report.get("rust_license_spdx_unresolved"),
            "unresolved/duplicate Rust SPDX")
    expected_lines = []
    total_notices = 0
    missing = set()
    for name in sorted(items):
        p = items[name]
        expr = p.get("license_spdx")
        require(isinstance(expr, str) and len(expr) < 180, "missing SPDX: " + name)
        license_ids = set(re.findall(r"[A-Za-z][A-Za-z0-9.-]*|0BSD", expr))
        require(license_ids - {"OR", "AND"} <= SPDX_IDS and
                bool(license_ids - {"OR", "AND"}), "unreviewed SPDX: " + name)
        files = p.get("notice_files")
        require(isinstance(files, list) and len(set(files)) == len(files),
                "invalid notice inventory: " + name)
        for fn in files:
            require(re.fullmatch(r"(?:LICENSE|LICENCE|COPYING|NOTICE|COPYRIGHT|AUTHORS)(?:[-._][A-Za-z0-9._-]+)?",
                                 fn, re.I) is not None,
                    "unreviewed notice filename: " + name)
            original_prefix = ("=== " + name + " / " + fn + " ===\n"
                               "SPDX: " + expr + "\n"
                               "SOURCE: https://crates.io/crates/"
                               + p["name"] + "/" + p["version"] + "\n"
                               "ORIGINAL_FILE_SHA256: ")
            start = license_bundle.find(original_prefix)
            require(start >= 0 and license_bundle.count(original_prefix) == 1,
                    "source NOTICE/license absent or duplicate: " + name + "/" + fn)
            tail = license_bundle[start + len(original_prefix):]
            require(re.match(r"[0-9a-f]{64}\n", tail) is not None,
                    "missing original file SHA: " + name + "/" + fn)
            recorded_hash = tail[:64]
            text_start = start + len(original_prefix) + 65
            text_end = license_bundle.find("\n=== ", text_start)
            if text_end == -1:
                text_end = license_bundle.find("\nEND_LICENSE_TEXTS", text_start)
            require(text_end >= text_start,
                    "unterminated source notice: " + name + "/" + fn)
            original = license_bundle[text_start:text_end].encode("utf-8")
            require(sha(original) == recorded_hash,
                    "altered original NOTICE/LICENSE SHA: " + name + "/" + fn)
            total_notices += 1
        if not files:
            missing.add(name)
        if name in COMPOUND:
            require(all(f in files for f in COMPOUND[name]),
                    "missing AND/Unicode third-party license: " + name)
        if name in MPL:
            require(expr == "MPL-2.0" and
                    any(f.lower().startswith("license") for f in files),
                    "MPL crate license absent: " + name)
        expected_lines.append(name + " | " + expr + " | notices=" +
                              (",".join(files) or "MISSING"))
    require(missing == EXPECTED_MISSING and
            set(report.get("rust_crate_notice_files_absent", [])) == missing and
            set(report.get("rust_source_package_notice_missing", [])) == missing,
            "unexpected missing standard crate notice filenames")
    require(total_notices == report.get("rust_source_notice_file_count") and
            total_notices >= 366, "missing source notices/copyright files")
    require(sha(license_bundle.encode("utf-8")) ==
            report.get("rust_source_license_bundle_sha256"),
            "source notices aggregate SHA mismatch")
    inventory = checked_file(root, "packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt").decode("utf-8")
    lines = inventory.split("BEGIN_COMPONENTS\n", 1)[1].split("\nEND_COMPONENTS", 1)[0].splitlines()
    require(lines == expected_lines, "pinned component inventory mismatch")
    require({(p["name"], p["version"]) for p in report.get("npm_production_packages", [])}
            == NPM and all(p.get("license") == "MIT"
                           for p in report["npm_production_packages"]),
            "npm production license/versions mismatch")
    for key in EXPECTED_MISSING:
        require(key in SUPPLEMENTS and checked_file(root, SUPPLEMENTS[key]),
                "unresolved supplemental license: " + key)


def verify_legal_evidence(root, audit_path, network=True):
    root = Path(root)
    check_documents(root)
    audit_file = Path(audit_path)
    require(audit_file.is_file() and not audit_file.is_symlink(),
            "missing source audit report")
    raw = audit_file.read_bytes()
    require(len(raw) < 3 * 1024 * 1024, "oversized source audit")
    report = json.loads(raw)
    bundle = checked_file(root, "packaging/licenses/rust-cli-source-license-texts.txt",
                          16 * 1024 * 1024).decode("utf-8")
    check_source_audit(root, report, bundle)
    if network:
        fetch_mpl_sources()
    return True


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--audit-report", type=Path, required=True)
    args = ap.parse_args()
    try:
        verify_legal_evidence(Path(__file__).resolve().parents[2], args.audit_report)
    except (OSError, UnicodeError, ValueError, KeyError,
            urllib.request.URLError) as exc:
        print("G4_2_LEGAL_COMPLIANCE=FAIL " + str(exc))
        raise SystemExit(1)
    print("G4_2_LEGAL_COMPLIANCE=PASS BINARY_PROVENANCE=NOT_VERIFIED")
