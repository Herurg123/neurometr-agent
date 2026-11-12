"""Unit tests for fail-closed, non-PR GitHub Actions artifact publication."""
import importlib.util
import unittest
from pathlib import Path

FILE = Path(__file__).with_name("publication_gate.py")
spec = importlib.util.spec_from_file_location("g4_publication_gate", FILE)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)

HEAD = "b" * 40

def complete():
    policy = dict(gate.REQUIRED_APPROVALS)
    policy["evidence"] = {
        "llmfit_tag": "v1.1.20",
        "llmfit_official_zip_sha256": gate.EXPECTED_SHA,
        "owner_rights_decision_ref": "approved-owner-decision",
        "rust_cli_sbom_sha256": "c" * 64,
        "rust_notices_sha256": "d" * 64,
        "contracts_catalog_rights_review_ref": "approved-license-review"
    }
    return policy

class PublicationGateTests(unittest.TestCase):
    def test_repository_policy_remains_blocked(self):
        policy = __import__("json").loads(
            (FILE.parents[2] / "packaging" / "public_artifact_gate.json").read_text(encoding="utf-8"))
        ok, reason = gate.evaluate(policy, "push", "refs/heads/main", HEAD, HEAD)
        self.assertFalse(ok)
        self.assertEqual(reason, "INCOMPLETE_REDISTRIBUTION_APPROVAL")

    def test_hypothetical_complete_policy_can_unlock_only_main(self):
        valid = complete()
        self.assertEqual(gate.evaluate(valid, "push", "refs/heads/main", HEAD, HEAD), (True, "APPROVED"))
        self.assertEqual(gate.evaluate(valid, "workflow_dispatch", "refs/heads/main", HEAD, HEAD), (True, "APPROVED"))

    def test_never_publish_from_pr_or_feature_branch(self):
        for event, ref in [
            ("pull_request", "refs/pull/9/merge"),
            ("pull_request", "refs/heads/main"),
            ("push", "refs/heads/feature"),
            ("workflow_dispatch", "refs/heads/feature"),
            ("schedule", "refs/heads/main"),
        ]:
            with self.subTest(event=event, ref=ref):
                ok, reason = gate.evaluate(complete(), event, ref, HEAD, HEAD)
                self.assertFalse(ok)
                self.assertEqual(reason, "NOT_MAIN_PUSH_OR_DISPATCH")

    def test_checkout_mismatch_or_incomplete_license_evidence(self):
        valid = complete()
        for checked in ("a" * 40, "invalid", ""):
            self.assertFalse(gate.evaluate(valid, "push", "refs/heads/main", HEAD, checked)[0])
        for key in ("agent_binary_redistribution_rights",
                    "llmfit_transitive_rust_licenses_and_notices",
                    "canonical_contract_catalog_distribution_rights",
                    "package_user_labeling"):
            bad = complete()
            bad[key] = "unverified"
            self.assertFalse(gate.evaluate(bad, "push", "refs/heads/main", HEAD, HEAD)[0])
        for key in ("owner_rights_decision_ref", "rust_cli_sbom_sha256",
                    "rust_notices_sha256", "contracts_catalog_rights_review_ref"):
            bad = complete()
            bad["evidence"].pop(key)
            self.assertFalse(gate.evaluate(bad, "push", "refs/heads/main", HEAD, HEAD)[0])

    def test_malformed_policy_stays_blocked(self):
        for malformed in (None, {}, [], {"status": True}, {"status": "approved"}, "approved"):
            self.assertFalse(gate.evaluate(malformed, "push", "refs/heads/main", HEAD, HEAD)[0])

if __name__ == "__main__":
    unittest.main(verbosity=2)
