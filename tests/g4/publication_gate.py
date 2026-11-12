"""Fail-closed G4.2 redistribution gate; never trust approval strings alone.

A future approval requires independently generated Windows CLI evidence AND
verified distribution documents. This script cannot confer a code license.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
EXPECTED_SHA = "1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489"
EXPECTED_COMMIT = "bb5771394e88e352c266f4bf8bdb7a5295a998a8"
HEX40 = re.compile(r"^[a-f0-9]{40}$")
HEX64 = re.compile(r"^[a-f0-9]{64}$")
REQUIRED_APPROVALS = {
    "schema":"neurometr.g4.2.public-artifact-gate.v1",
    "status":"approved",
    "owner_authorization_for_public_ci_artifacts":"granted",
    "agent_binary_redistribution_rights":"confirmed",
    "llmfit_v1_1_20_mit_license":"verified",
    "llmfit_transitive_rust_licenses_and_notices":"audited_and_bundled",
    "canonical_contract_catalog_distribution_rights":"confirmed",
    "package_user_labeling":"user_qa_distribution_ready",
    "release_manifest_is_signature":False,
}
DOCUMENTS = {
    "owner_rights_decision_ref":"packaging/DISTRIBUTION_TERMS_RU.txt",
    "contracts_catalog_rights_review_ref":"packaging/DATA_SOURCE_ATTRIBUTION_RU.txt",
    "llmfit_audit_scope_ref":"packaging/LLMFIT_WINDOWS_CLI_AUDIT_RU.txt",
}
def read_bounded(path, limit=2*1024*1024):
    if not path.is_file() or path.is_symlink() or path.stat().st_size > limit:
        raise ValueError("missing or unsafe evidence")
    return path.read_bytes()
def digest(b):
    return hashlib.sha256(b).hexdigest()
def valid_source_evidence(policy, audit_path, root):
    ev=policy.get("evidence")
    if not isinstance(ev,dict) or ev.get("llmfit_tag")!="v1.1.20" or \
       ev.get("llmfit_upstream_commit")!=EXPECTED_COMMIT or \
       ev.get("llmfit_official_zip_sha256")!=EXPECTED_SHA:
        return False
    for field,path in DOCUMENTS.items():
        if ev.get(field)!=path:
            return False
        try:
            document=read_bounded(root/path).decode("utf-8")
        except (OSError,UnicodeError,ValueError):
            return False
        if len(document)<300 or "neurometr" not in path.lower() and "НЕЙРОМЕТР" not in document:
            return False
    for field in ("rust_windows_cli_sbom_sha256","rust_complete_license_notices_sha256",
                  "npm_embedded_asset_inventory_sha256"):
        if not isinstance(ev.get(field),str) or not HEX64.fullmatch(ev[field]):
            return False
    # The reviewed, committed inventory must equal fresh pinned Cargo evidence
    # produced on the Windows runner, not a user-provided policy hash.
    if audit_path is None:
        return False
    try:
        audit_raw=read_bounded(Path(audit_path),3*1024*1024)
        report=json.loads(audit_raw)
        official_inventory=read_bounded(root/"packaging"/"audit"/"llmfit_windows_cli_sbom.json",3*1024*1024)
        notices=read_bounded(root/"packaging"/"licenses"/"rust-cli-complete-notices.txt",2*1024*1024)
        npm=read_bounded(root/"packaging"/"audit"/"llmfit_embedded_npm_assets.json",3*1024*1024)
    except (OSError,ValueError):
        return False
    if digest(official_inventory)!=ev["rust_windows_cli_sbom_sha256"] or \
       digest(notices)!=ev["rust_complete_license_notices_sha256"] or \
       digest(npm)!=ev["npm_embedded_asset_inventory_sha256"]:
        return False
    if audit_raw!=official_inventory:
        return False
    if report.get("target")!="x86_64-pc-windows-msvc" or \
       report.get("upstream_commit")!=EXPECTED_COMMIT or \
       report.get("feature_scope")!="default" or \
       report.get("cargo_edges")!="normal,build":
        return False
    if not report.get("full_rust_license_texts_in_package") or \
       not report.get("official_exe_compiled_graph_attested") or \
       not report.get("official_exe_embedded_web_assets_attested"):
        return False
    if report.get("rust_license_spdx_unresolved") or report.get("rust_crate_notice_files_absent"):
        return False
    crates=report.get("rust_packages")
    if not isinstance(crates,list) or len(crates)<20 or \
       any(not isinstance(c,dict) or not c.get("license_spdx") for c in crates):
        return False
    # npm provenance is also reviewed explicitly; arbitrary strings are
    # insufficient to certify what upstream embedded in the signed EXE.
    try:
        npm_data=json.loads(npm)
    except ValueError:
        return False
    return npm_data.get("official_signed_exe_assets_independently_attested") is True

def evaluate(policy,event,ref,source_sha,checked_sha,audit_path=None,root=ROOT):
    if event not in ("push","workflow_dispatch") or ref!="refs/heads/main":
        return False,"NOT_MAIN_PUSH_OR_DISPATCH"
    if not HEX40.fullmatch(source_sha or "") or source_sha!=checked_sha:
        return False,"CHECKOUT_PROVENANCE_MISMATCH"
    if not isinstance(policy,dict):
        return False,"POLICY_MISSING"
    for field,val in REQUIRED_APPROVALS.items():
        if type(policy.get(field)) is not type(val) or policy.get(field)!=val:
            return False,"INCOMPLETE_REDISTRIBUTION_APPROVAL"
    if not valid_source_evidence(policy,audit_path,root):
        return False,"REQUIRED_INDEPENDENT_LICENSE_EVIDENCE_MISSING"
    return True,"APPROVED"

def run():
    ap=argparse.ArgumentParser()
    ap.add_argument("--policy",type=Path,required=True)
    ap.add_argument("--output",type=Path)
    ap.add_argument("--rust-audit-report",type=Path)
    opts=ap.parse_args()
    try:
        data=read_bounded(opts.policy,16384)
        policy=json.loads(data)
    except (OSError,ValueError):
        policy=None
    allowed,code=evaluate(policy,os.environ.get("GITHUB_EVENT_NAME",""),
                          os.environ.get("GITHUB_REF",""),
                          os.environ.get("G4_EXPECTED_SOURCE_COMMIT",""),
                          os.environ.get("G4_VERIFIED_CHECKOUT_COMMIT",""),
                          opts.rust_audit_report)
    if opts.output is not None:
        with opts.output.open("a",encoding="utf-8",newline="\n") as out:
            out.write("approved="+("true" if allowed else "false")+"\n")
            out.write("reason="+code+"\n")
    print("G4_2_PUBLIC_ARTIFACT_GATE="+("APPROVED" if allowed else "BLOCKED")+" REASON="+code)
    return 0

if __name__=="__main__":
    raise SystemExit(run())
