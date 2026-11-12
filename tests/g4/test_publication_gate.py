"""Negative G4.2 policy tests. Network forbidden in tests, real CI audits sources."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
from unittest import mock
import unittest

FILE = Path(__file__).with_name("publication_gate.py")
sys.path.insert(0, str(FILE.parent))
spec = importlib.util.spec_from_file_location("g4_publication_gate", FILE)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
HEAD = "b" * 40
POLICY = json.loads((FILE.parents[2] / "packaging" /
                     "public_artifact_gate.json").read_text(encoding="utf-8"))


class PublicationGateTests(unittest.TestCase):
    def test_pr_branch_and_non_main_never_publish_even_with_verified_licenses(self):
        with mock.patch.object(gate, "policy_approved", return_value=True), \
             mock.patch.object(gate, "verify_legal_evidence", return_value=True):
            for event, ref in (
                ("pull_request", "refs/pull/9/merge"),
                ("pull_request", "refs/heads/main"),
                ("push", "refs/heads/feature"),
                ("workflow_dispatch", "refs/heads/feature"),
                ("schedule", "refs/heads/main"),
            ):
                self.assertEqual(gate.evaluate(POLICY, event, ref, HEAD, HEAD,
                                               audit_path=Path("test-audit.json")),
                                 (False, "NOT_MAIN_PUSH_OR_DISPATCH"))
    def test_positive_main_requires_independent_legal_check_but_not_binary_attestation(self):
        with mock.patch.object(gate, "policy_approved", return_value=True), \
             mock.patch.object(gate, "verify_legal_evidence", return_value=True) as verify:
            self.assertEqual(gate.evaluate(POLICY, "push", "refs/heads/main", HEAD, HEAD,
                                           audit_path=Path("source.json"), network=False),
                             (True, "APPROVED"))
            verify.assert_called_once()
    def test_missing_or_negative_legal_evidence_blocks(self):
        with mock.patch.object(gate, "policy_approved", return_value=True):
            self.assertEqual(gate.evaluate(POLICY, "push", "refs/heads/main", HEAD, HEAD),
                             (False, "MANDATORY_LICENSE_NOTICE_OR_SOURCE_CHECK_FAILED"))
            with mock.patch.object(gate, "verify_legal_evidence",
                                   side_effect=ValueError("MPL source missing")):
                self.assertFalse(gate.evaluate(POLICY, "push", "refs/heads/main",
                                               HEAD, HEAD, audit_path=Path("x"))[0])
    def test_owner_risk_acceptance_and_not_verified_provenance_are_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel in gate.DOCUMENTS.values():
                p = root / rel
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text("evidence", encoding="utf-8")
            self.assertTrue(gate.policy_approved(POLICY, root))
            for key, value in (
                ("owner_risk_acceptance", None),
                ("owner_risk_acceptance", "PENDING"),
                ("binary_provenance", "PASS"),
                ("binary_provenance", "ATTESTED"),
                ("legal_compliance", "PASS"),
                ("status", "blocked"),
                ("release_manifest_is_signature", True),
            ):
                bad = dict(POLICY)
                bad[key] = value
                self.assertFalse(gate.policy_approved(bad, root), (key, value))
            bad = json.loads(json.dumps(POLICY))
            bad["evidence"]["owner_risk_acceptance_ref"] = "not-the-owner.txt"
            self.assertFalse(gate.policy_approved(bad, root))
            (root / gate.DOCUMENTS["owner_risk_acceptance_ref"]).unlink()
            self.assertFalse(gate.policy_approved(POLICY, root))
    def test_pinned_zip_and_source_sha_cannot_be_replaced(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel in gate.DOCUMENTS.values():
                p = root / rel
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text("test", encoding="utf-8")
            for key in ("llmfit_official_zip_sha256", "llmfit_upstream_commit"):
                bad = json.loads(json.dumps(POLICY))
                bad["evidence"][key] = "0" * 64 if key.endswith("sha256") else "0" * 40
                self.assertFalse(gate.policy_approved(bad, root))
            with mock.patch.object(gate, "policy_approved", return_value=True):
                for mismatch in ("a" * 40, "invalid", ""):
                    self.assertEqual(gate.evaluate(POLICY, "push", "refs/heads/main",
                                                   HEAD, mismatch),
                                     (False, "CHECKOUT_PROVENANCE_MISMATCH"))
    def test_bogus_policy_never_approves(self):
        for bad in (None, {}, [], {"status": "approved"}, "approved"):
            self.assertFalse(gate.policy_approved(bad))
    def test_missing_supplemental_document_blocks(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel in gate.DOCUMENTS.values():
                p = root / rel
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text("test", encoding="utf-8")
            self.assertTrue(gate.policy_approved(POLICY, root))
            (root / "packaging" / "LLMFIT_WINDOWS_CLI_COMPONENTS_RU.txt").unlink()
            self.assertFalse(gate.policy_approved(POLICY, root))


if __name__ == "__main__":
    unittest.main(verbosity=2)
