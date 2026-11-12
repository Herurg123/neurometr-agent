"""Strict G4.2 gate: legal compliance verified independently of EXE provenance.

The owner accepted only the explicit NOT_VERIFIED binary provenance for QA.
No upload from PR or feature branch. Main upload requires real CI evidence,
correct pinned source, and user-facing OSS notices + corresponding MPL source.
"""
import argparse
import json
import os
from pathlib import Path
import re

from legal_compliance import verify_legal_evidence

ROOT = Path(__file__).resolve().parents[2]
EXPECTED_SHA = "1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489"
EXPECTED_COMMIT = "bb5771394e88e352c266f4bf8bdb7a5295a998a8"
HEX40 = re.compile(r"^[a-f0-9]{40}$")
REQUIRED_APPROVALS = {
    "schema": "neurometr.g4.2.public-artifact-gate.v2",
    "status": "approved_for_main_ci_only",
    "owner_authorization_for_public_ci_artifacts": "granted",
    "agent_binary_redistribution_rights": "confirmed",
    "llmfit_v1_1_20_mit_license": "verified",
    "legal_compliance": "pass_if_live_ci_evidence_verified",
    "binary_provenance": "NOT_VERIFIED",
    "owner_risk_acceptance": "APPROVED",
    "canonical_contract_catalog_distribution_rights": "confirmed",
    "package_user_labeling": "user_qa_distribution_ready",
    "release_manifest_is_signature": False,
}
DOCUMENTS = {
    "owner_rights_decision_ref": "packaging/DISTRIBUTION_TERMS_RU.txt",
    "owner_risk_acceptance_ref": "packaging/G4_2_OWNER_RISK_ACCEPTANCE_RU.txt",
    "contracts_catalog_rights_review_ref": "packaging/DATA_SOURCE_ATTRIBUTION_RU.txt",
    "llmfit_audit_scope_ref": "packaging/LLMFIT_WINDOWS_CLI_AUDIT_RU.txt",
    "rust_windows_cli_inventory_ref": "packaging/LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt",
    "rust_license_notices_bundle_ref": "packaging/licenses/rust-cli-source-license-texts.txt",
}


def policy_approved(policy, root=ROOT):
    if not isinstance(policy, dict):
        return False
    for k, v in REQUIRED_APPROVALS.items():
        if type(policy.get(k)) is not type(v) or policy.get(k) != v:
            return False
    e = policy.get("evidence")
    if not isinstance(e, dict) or e.get("llmfit_tag") != "v1.1.20" or \
       e.get("llmfit_upstream_commit") != EXPECTED_COMMIT or \
       e.get("llmfit_official_zip_sha256") != EXPECTED_SHA or \
       e.get("rust_windows_cli_candidate_count") != 215 or \
       e.get("npm_packages_in_source_lockfile") != 5 or \
       e.get("independent_official_exe_compiled_graph_attestation") != "NOT_VERIFIED" or \
       e.get("independent_official_exe_embedded_js_attestation") != "NOT_VERIFIED":
        return False
    for key, path in DOCUMENTS.items():
        if e.get(key) != path:
            return False
        p = root / path
        if not p.is_file() or p.is_symlink() or p.stat().st_size <= 0:
            return False
    return True


def check_legal(policy, audit_path, root=ROOT, network=True):
    if not policy_approved(policy, root) or audit_path is None:
        return False
    try:
        return verify_legal_evidence(root, audit_path, network=network)
    except (OSError, UnicodeError, ValueError, KeyError, RuntimeError):
        return False


def evaluate(policy, event, ref, source_sha, checked_sha, audit_path=None,
             root=ROOT, network=True):
    # Event/ref block is evaluated first; PR must NEVER upload, even with full evidence.
    if event not in ("push", "workflow_dispatch") or ref != "refs/heads/main":
        return False, "NOT_MAIN_PUSH_OR_DISPATCH"
    if not HEX40.fullmatch(source_sha or "") or source_sha != checked_sha:
        return False, "CHECKOUT_PROVENANCE_MISMATCH"
    if not policy_approved(policy, root):
        return False, "INCOMPLETE_REDISTRIBUTION_APPROVAL"
    if not check_legal(policy, audit_path, root, network=network):
        return False, "MANDATORY_LICENSE_NOTICE_OR_SOURCE_CHECK_FAILED"
    return True, "APPROVED"


def run():
    ap = argparse.ArgumentParser()
    ap.add_argument("--policy", type=Path, required=True)
    ap.add_argument("--output", type=Path)
    ap.add_argument("--rust-audit-report", type=Path)
    opts = ap.parse_args()
    try:
        p = opts.policy
        if p.is_symlink() or p.stat().st_size > 16384:
            raise ValueError("unsafe gate policy")
        policy = json.loads(p.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        policy = None

    # On PR still check legal evidence; PR's artifact gate ALWAYS stays blocked.
    legal = check_legal(policy, opts.rust_audit_report)
    approved, code = evaluate(
        policy, os.environ.get("GITHUB_EVENT_NAME", ""), os.environ.get("GITHUB_REF", ""),
        os.environ.get("G4_EXPECTED_SOURCE_COMMIT", ""),
        os.environ.get("G4_VERIFIED_CHECKOUT_COMMIT", ""),
        opts.rust_audit_report, network=False if legal else True
    ) if legal else (False, "MANDATORY_LICENSE_NOTICE_OR_SOURCE_CHECK_FAILED")
    if os.environ.get("GITHUB_EVENT_NAME") not in ("push", "workflow_dispatch") or \
       os.environ.get("GITHUB_REF") != "refs/heads/main":
        approved, code = False, "NOT_MAIN_PUSH_OR_DISPATCH"
    if opts.output is not None:
        with opts.output.open("a", encoding="utf-8", newline="\n") as out:
            out.write("approved=" + ("true" if approved else "false") + "\n")
            out.write("reason=" + code + "\n")
            out.write("legal_compliance=" + ("PASS" if legal else "FAIL") + "\n")
            out.write("binary_provenance=NOT_VERIFIED\n")
            out.write("owner_risk_acceptance=" +
                      ("APPROVED" if policy_approved(policy) else "NOT_APPROVED") + "\n")
    print("G4_2_LEGAL_COMPLIANCE=" + ("PASS" if legal else "FAIL"))
    print("G4_2_BINARY_PROVENANCE=NOT_VERIFIED")
    print("G4_2_OWNER_RISK_ACCEPTANCE=" +
          ("APPROVED" if policy_approved(policy) else "NOT_APPROVED"))
    print("G4_2_PUBLIC_ARTIFACT_GATE=" + ("APPROVED" if approved else "BLOCKED") +
          " REASON=" + code)
    return 0


if __name__ == "__main__":
    raise SystemExit(run())
