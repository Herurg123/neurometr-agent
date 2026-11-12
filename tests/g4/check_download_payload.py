"""Validate exactly the two G4.2 user-download payloads in private Windows CI.

The GitHub-generated outer artifact ZIP is NOT the Neurometr application ZIP.
No remote uploads, screenshots, runner hardware reports or tokens are produced.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import stat
import zipfile

ROOT = "Neurometr-Windows-x64/"
EXPECTED_LL = "1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489"
EXPECTED_CAT = "1d84ff596c589a4db004f8a3fa17490d5479f57db3d7b0a433a7e448289651d2"
SHA = re.compile(r"[a-f0-9]{40}\Z")
BAD = re.compile(r"(?i)(?:^|/)(?:\.git|\.env|secret[s]?|token[s]?|credential[s]?|private|__pycache__|node_modules|\.github|runner|g4-test|test-output)(?:/|$)")
def sha(data):
    return hashlib.sha256(data).hexdigest()

def verify(folder, commit, public=False):
    assert SHA.fullmatch(commit), "invalid source commit"
    files = sorted(p.name for p in folder.iterdir())
    expected_zip = f"Neurometr-Windows-x64-{commit}.zip"
    assert files == ["SHA256SUMS.txt", expected_zip] or files == sorted(["SHA256SUMS.txt", expected_zip]), "unexpected user download files"
    zip_path = folder / expected_zip
    raw = zip_path.read_bytes()
    checksum = (folder / "SHA256SUMS.txt").read_text(encoding="ascii")
    assert checksum == f"{sha(raw)} *{expected_zip}\n", "sidecar digest does not match application ZIP"
    assert 1024 < len(raw) < 200 * 1024 * 1024, "unsafe ZIP size"
    with zipfile.ZipFile(zip_path) as archive:
        infos = archive.infolist()
        names = [i.filename for i in infos]
        assert len(names) == len(set(n.lower() for n in names)), "duplicate ZIP path"
        assert len(names) <= 110 and len(names) >= 26, "unexpected inventory count"
        assert all(n.startswith(ROOT) and not BAD.search(n) and ".." not in n and "\\" not in n
                   and not n.startswith("/") and not n.endswith("/") for n in names), "unsafe or unapproved file path"
        assert all(stat.S_IFMT(i.external_attr >> 16) in (0, stat.S_IFREG) for i in infos), "special ZIP entry"
        manifest = json.loads(archive.read(ROOT + "release-manifest.json"))
        assert manifest["agent_source_commit"] == commit, "mismatched source commit"
        assert manifest["catalog_sha256"] == EXPECTED_CAT, "catalog identity mismatch"
        assert manifest["llmfit_official_archive_sha256"] == EXPECTED_LL, "upstream identity mismatch"
        assert manifest["llmfit_version"] == "1.1.20", "CLI version drift"
        entries = manifest["files"]
        assert len(entries) + 1 == len(names), "hidden extra file"
        expected = sorted(ROOT + e["path"] for e in entries) + [ROOT + "release-manifest.json"]
        assert set(names) == set(expected), "inventory paths differ"
        for e in entries:
            content = archive.read(ROOT + e["path"])
            assert len(content) == e["bytes"] and sha(content) == e["sha256"], "per-file integrity failed"
        if public:
            assert manifest["format"] == "neurometr.g4.2.test-inventory", "unexpected test inventory format"
            assert manifest["distribution"] == "user-qa-distribution", "package still marked internal-only"
            necessary = ("README_RU.txt", "THIRD_PARTY_NOTICES_RU.txt",
                         "DISTRIBUTION_TERMS_RU.txt", "G4_2_OWNER_RISK_ACCEPTANCE_RU.txt",
                         "DATA_SOURCE_ATTRIBUTION_RU.txt",
                         "LLMFIT_WINDOWS_CLI_AUDIT_RU.txt")
            for n in (ROOT + x for x in necessary):
                text = archive.read(n).decode("utf-8")
                assert "не разрешён к распространению" not in text.lower()
                assert "не для публичного распространения" not in text.lower()
                assert "internal-ci-only" not in text.lower()
            owner = archive.read(ROOT + "G4_2_OWNER_RISK_ACCEPTANCE_RU.txt").decode("utf-8")
            assert "BINARY_PROVENANCE = NOT_VERIFIED" in owner, "EXE provenance falsely attested"
            assert "OWNER_RISK_ACCEPTANCE = APPROVED" in owner, "missing owner risk acceptance"
            notices = archive.read(ROOT + "THIRD_PARTY_NOTICES_RU.txt").decode("utf-8")
            assert "Mozilla Public License" in notices and "§3.2" in notices, "MPL source notice missing"
            for url, digest in (
                ("https://static.crates.io/crates/colored/colored-3.1.1.crate",
                 "faf9468729b8cbcea668e36183cb69d317348c2e08e994829fb56ebfdfbaac34"),
                ("https://static.crates.io/crates/option-ext/option-ext-0.2.0.crate",
                 "04744f49eae99ab78e0d5c0b603ab218f515ea8cfe5a456d7629ad883a3b6e7d"),
            ):
                assert url in notices and digest in notices, "missing pinned MPL corresponding source"
            assert "прав" in archive.read(ROOT + "DISTRIBUTION_TERMS_RU.txt").decode("utf-8")
            assert "rmcp" in notices and "Unicode-3.0" in notices
            licenses = ("react-family-MIT.txt", "js-tokens-MIT.txt", "loose-envify-MIT.txt")
            for filename in licenses:
                lic = archive.read(ROOT + "LICENSES/" + filename).decode("utf-8")
                assert "Permission is hereby granted" in lic and "Copyright" in lic, "incomplete npm license"
            clipboard_text = archive.read(ROOT + "LICENSES/clipboard-win-BSL-1.0.txt").decode("utf-8")
            assert "Boost Software License - Version 1.0" in clipboard_text
            assert "Permission is hereby granted" in clipboard_text
            rmcp_text = archive.read(ROOT + "LICENSES/rmcp-3.5.1-upstream-LICENSE.txt").decode("utf-8")
            assert "licensing transition from the MIT License to the Apache License" in rmcp_text
            assert "END OF TERMS AND CONDITIONS" in rmcp_text and "MIT License" in rmcp_text
            rust_text = archive.read(ROOT + "LICENSES/rust-cli-source-license-texts.txt").decode("utf-8")
            assert "BEGIN_LICENSE_TEXTS" in rust_text and "END_LICENSE_TEXTS" in rust_text
            assert "colored@3.1.1" in rust_text and "option-ext@0.2.0" in rust_text
            assert rust_text.count("ORIGINAL_FILE_SHA256: ") >= 200, "incomplete source crate licenses"
            assert "MISSING_STANDARD_NOTICE_FILE_NAMES:" in rust_text
            boost = archive.read(ROOT + "LICENSES/clipboard-win-BSL-1.0.txt").decode("utf-8")
            rmcp = archive.read(ROOT + "LICENSES/rmcp-3.5.1-upstream-LICENSE.txt").decode("utf-8")
            assert "Boost Software License - Version 1.0" in boost
            assert "licensing transition" in rmcp and "MIT License" in rmcp and "Apache License" in rmcp
            apache = archive.read(ROOT + "LICENSES/jsonschema-Apache-2.0.txt").decode("utf-8")
            assert "END OF TERMS AND CONDITIONS" in apache and "APPENDIX: How to apply" in apache, "Apache-2.0 legal text incomplete"
            assert "Copyright 2024 Alibaba Cloud" in archive.read(ROOT + "DATA_SOURCE_ATTRIBUTION_RU.txt").decode("utf-8")
            assert "Qwen2.5-0.5B" in archive.read(ROOT + "DATA_SOURCE_ATTRIBUTION_RU.txt").decode("utf-8")
            assert "Qwen2.5-1.5B" in archive.read(ROOT + "DATA_SOURCE_ATTRIBUTION_RU.txt").decode("utf-8")
            assert b"internal-ci-only" not in archive.read(ROOT + "release-manifest.json"), "internal-only manifest leaked"
    return len(raw), sha(raw), len(entries)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--dir", type=Path, required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--public", action="store_true")
    args = parser.parse_args()
    size, checksum, count = verify(args.dir, args.commit, public=args.public)
    print(f"G4_2_DOWNLOAD_PAYLOAD=PASS ZIP_BYTES={size} ZIP_SHA256={checksum} INVENTORY_FILES={count} MODE={'PUBLIC' if args.public else 'DRY_RUN'}")
