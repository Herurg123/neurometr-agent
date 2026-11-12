"""The public gate must reject policy strings without real independent license evidence."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

FILE=Path(__file__).with_name("publication_gate.py")
spec=importlib.util.spec_from_file_location("g4_publication_gate",FILE)
gate=importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
HEAD="b"*40

def forged_complete():
    p=dict(gate.REQUIRED_APPROVALS)
    p["evidence"]={
      "llmfit_tag":"v1.1.20",
      "llmfit_upstream_commit":gate.EXPECTED_COMMIT,
      "llmfit_official_zip_sha256":gate.EXPECTED_SHA,
      **gate.DOCUMENTS,
      "rust_windows_cli_sbom_sha256":"c"*64,
      "rust_complete_license_notices_sha256":"d"*64,
      "npm_embedded_asset_inventory_sha256":"e"*64,
    }
    return p

class PublicationGateTests(unittest.TestCase):
    def test_current_repository_remains_blocked_for_unresolved_rust_notices(self):
        p=json.loads((FILE.parents[2]/"packaging"/"public_artifact_gate.json").read_text(encoding="utf-8"))
        self.assertEqual(p["owner_authorization_for_public_ci_artifacts"],"granted")
        self.assertEqual(p["agent_binary_redistribution_rights"],"confirmed")
        self.assertEqual(p["canonical_contract_catalog_distribution_rights"],"confirmed")
        self.assertEqual(gate.evaluate(p,"push","refs/heads/main",HEAD,HEAD),
                         (False,"INCOMPLETE_REDISTRIBUTION_APPROVAL"))
    def test_forged_all_approved_strings_and_fake_sha_do_not_unlock(self):
        p=forged_complete()
        for event in ("push","workflow_dispatch"):
            self.assertEqual(gate.evaluate(p,event,"refs/heads/main",HEAD,HEAD),
                             (False,"REQUIRED_INDEPENDENT_LICENSE_EVIDENCE_MISSING"))
        with tempfile.TemporaryDirectory() as folder:
            f=Path(folder)/"audit.json"
            f.write_text('{"status":"approved"}',encoding="utf-8")
            self.assertFalse(gate.evaluate(p,"push","refs/heads/main",HEAD,HEAD,audit_path=f)[0])
    def test_never_publish_from_pr_feature_other_event(self):
        for event,ref in (("pull_request","refs/pull/9/merge"),
                          ("pull_request","refs/heads/main"),
                          ("push","refs/heads/feature"),
                          ("workflow_dispatch","refs/heads/feature"),
                          ("schedule","refs/heads/main")):
            self.assertEqual(gate.evaluate(forged_complete(),event,ref,HEAD,HEAD),
                             (False,"NOT_MAIN_PUSH_OR_DISPATCH"))
    def test_unverified_checkout_is_never_approved(self):
        for mismatch in ("a"*40,"invalid",""):
            self.assertEqual(gate.evaluate(forged_complete(),"push","refs/heads/main",HEAD,mismatch),
                             (False,"CHECKOUT_PROVENANCE_MISMATCH"))
    def test_malformed_or_missing_evidence_never_approves(self):
        for malformed in (None,{},[],{"status":True},{"status":"approved"},"approved"):
            self.assertFalse(gate.evaluate(malformed,"push","refs/heads/main",HEAD,HEAD)[0])
        for key in ("owner_rights_decision_ref","contracts_catalog_rights_review_ref",
                    "llmfit_audit_scope_ref","rust_windows_cli_sbom_sha256",
                    "rust_complete_license_notices_sha256","npm_embedded_asset_inventory_sha256"):
            p=forged_complete()
            p["evidence"].pop(key)
            self.assertFalse(gate.evaluate(p,"push","refs/heads/main",HEAD,HEAD)[0])
    def test_source_documents_are_bound_to_expected_paths(self):
        p=forged_complete()
        p["evidence"]["owner_rights_decision_ref"]="../secret"
        self.assertFalse(gate.evaluate(p,"push","refs/heads/main",HEAD,HEAD)[0])

if __name__=="__main__":
    unittest.main(verbosity=2)
