import hashlib
import json
import unittest
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, FormatChecker


ROOT = Path(__file__).resolve().parents[1]


def resolve_local_ref(document, ref):
    if not ref.startswith("#/"):
        raise AssertionError(f"external reference is not allowed: {ref}")
    value = document
    for part in ref[2:].split("/"):
        value = value[part.replace("~1", "/").replace("~0", "~")]
    return value


class ContractTests(unittest.TestCase):
    def test_admin_openapi_is_31_and_all_local_refs_resolve(self):
        document = yaml.safe_load((ROOT / "openapi" / "admin.yaml").read_text(encoding="utf-8"))
        self.assertEqual(document["openapi"], "3.1.0")
        self.assertEqual(
            set(document["paths"]),
            {
                "/object-upload-intents",
                "/me/notice-email-subscription",
                "/admin/ui-config",
                "/admin/users/{id}/sessions/revoke",
                "/admin/dashboard-snapshots/latest",
                "/admin/action-items",
                "/admin/metric-series",
                "/admin/notices",
                "/admin/notices/{id}/versions",
                "/admin/notice-import-jobs",
                "/admin/school-notices",
                "/admin/notices/{id}/reviews",
                "/admin/mail-operations",
                "/admin/mail-deliveries/{id}/retries",
                "/admin/mail-dead-letters/{id}/replays",
                "/admin/mail-suppressions",
                "/admin/mail-suppressions/{id}",
                "/admin/platform-feedback",
                "/admin/feedback-operations",
                "/admin/platform-feedback/{id}",
                "/admin/quiz-feedback/{id}/verifications",
                "/admin/quiz-feedback/{id}/resolutions",
                "/admin/food-operations",
                "/admin/food-entries/{id}/adjustments",
                "/admin/food-submissions/{id}/approvals",
                "/admin/food-vote-anomalies/{id}/resolutions",
                "/admin/system-operations",
                "/internal/admin-summaries/current",
                "/internal/action-items",
                "/internal/mail-deliveries",
            },
        )
        operation_ids = []

        def visit(value):
            if isinstance(value, dict):
                if "$ref" in value:
                    resolve_local_ref(document, value["$ref"])
                if "operationId" in value:
                    operation_ids.append(value["operationId"])
                for child in value.values():
                    visit(child)
            elif isinstance(value, list):
                for child in value:
                    visit(child)

        visit(document)
        self.assertEqual(len(operation_ids), len(set(operation_ids)))
        self.assertEqual(
            document["components"]["schemas"]["DomainCode"]["enum"],
            ["users", "notice", "mail", "feedback", "food", "system"],
        )
        self.assertEqual(document["components"]["schemas"]["Urgency"]["enum"], ["urgent", "normal"])

    def test_notice_schema_accepts_positive_and_rejects_negative_example(self):
        schema = json.loads((ROOT / "schemas" / "campus-notice-import.schema.json").read_text(encoding="utf-8"))
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema, format_checker=FormatChecker())
        valid = json.loads((ROOT / "examples" / "campus-notice.valid.json").read_text(encoding="utf-8"))
        invalid = json.loads((ROOT / "examples" / "campus-notice.invalid.json").read_text(encoding="utf-8"))
        self.assertEqual(list(validator.iter_errors(valid)), [])
        self.assertGreaterEqual(len(list(validator.iter_errors(invalid))), 5)
        self.assertEqual(hashlib.sha256(valid["body"].encode("utf-8")).hexdigest(), valid["content_sha256"])


if __name__ == "__main__":
    unittest.main()
