"""Fail-closed G4.2 GitHub Actions publication gate.

This is NOT a legal opinion: the approval policy documents the owner's actual
rights/notices review. 'granted' for public artifact visibility is insufficient
to approve redistribution. Every workflow run still checks event/ref/source.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re

EXPECTED_SHA = "1f848ddf2554adb89b3a31a034245826694148249fadace572c4544bb3c9c489"
HEX40 = re.compile(r"^[a-f0-9]{40}$")
HEX64 = re.compile(r"^[a-f0-9]{64}$")

REQUIRED_APPROVALS = {
    "schema": "neurometr.g4.2.public-artifact-gate.v1",
    "status": "approved",
    "owner_authorization_for_public_ci_artifacts": "granted",
    "agent_binary_redistribution_rights": "confirmed",
    "llmfit_v1_1_20_mit_license": "verified",
    "llmfit_transitive_rust_licenses_and_notices": "audited_and_bundled",
    "canonical_contract_catalog_distribution_rights": "confirmed",
    "package_user_labeling": "user_qa_distribution_ready",
    "release_manifest_is_signature": False,
}

def evaluate(policy, event, ref, source_sha, checked_sha):
    """Return (bool, generic code), never echo sensitive metadata."""
    if event not in ("push", "workflow_dispatch") or ref != "refs/heads/main":
        return False, "NOT_MAIN_PUSH_OR_DISPATCH"
    if not HEX40.fullmatch(source_sha or "") or source_sha != checked_sha:
        return False, "CHECKOUT_PROVENANCE_MISMATCH"
    if not isinstance(policy, dict):
        return False, "POLICY_MISSING"
    for field, value in REQUIRED_APPROVALS.items():
        if type(policy.get(field)) != type(value) or policy.get(field) != value:
            return False, "INCOMPLETE_REDISTRIBUTION_APPROVAL"
    evidence = policy.get("evidence")
    if not isinstance(evidence, dict):
        return False, "EVIDENCE_MISSING"
    if evidence.get("llmfit_tag") != "v1.1.20" or evidence.get("llmfit_official_zip_sha256") != EXPECTED_SHA:
        return False, "OFFICIAL_LLMFIT_SOURCE_MISMATCH"
    # Evidence must be fixed to a reviewed inventory and signed-off notices.
    for field in ("owner_rights_decision_ref", "rust_cli_sbom_sha256",
                  "rust_notices_sha256", "contracts_catalog_rights_review_ref"):
        value = evidence.get(field)
        if not isinstance(value, str) or not value.strip() or len(value) > 200:
            return False, "REQUIRED_AUDIT_EVIDENCE_MISSING"
    if not HEX64.fullmatch(evidence["rust_cli_sbom_sha256"]) or not HEX64.fullmatch(evidence["rust_notices_sha256"]):
        return False, "AUDIT_EVIDENCE_CHECKSUM_INVALID"
    return True, "APPROVED"

def run():
    p = argparse.ArgumentParser()
    p.add_argument("--policy", type=Path, required=True)
    p.add_argument("--output", type=Path)
    args = p.parse_args()
    try:
        body = args.policy.read_bytes()
        if len(body) > 16384:
            raise ValueError("policy oversized")
        policy = json.loads(body)
    except (OSError, ValueError):
        policy = None
    allowed, code = evaluate(
        policy,
        os.environ.get("GITHUB_EVENT_NAME", ""),
        os.environ.get("GITHUB_REF", ""),
        os.environ.get("G4_EXPECTED_SOURCE_COMMIT", ""),
        os.environ.get("G4_VERIFIED_CHECKOUT_COMMIT", ""),
    )
    if args.output is not None:
        with args.output.open("a", encoding="utf-8", newline="\n") as out:
            out.write("approved=" + ("true" if allowed else "false") + "\n")
            out.write("reason=" + code + "\n")
    print("G4_2_PUBLIC_ARTIFACT_GATE=" + ("APPROVED" if allowed else "BLOCKED") + " REASON=" + code)
    # A blocked public artifact must NEVER fail the full private CI smoke:
    # the workflow still verifies the package, but upload is skipped.
    return 0

if __name__ == "__main__":
    raise SystemExit(run())
