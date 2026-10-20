"""A fixed successor scope must preserve the original Food boundary."""
import importlib.util
from pathlib import Path
import unittest


SOURCE = Path(__file__).resolve().parents[1] / "deploy-henukit-food-scoped.py"
spec = importlib.util.spec_from_file_location("scoped_food", SOURCE)
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)


class PracticeFixture(engine.Release):
    services = ("portal", "portal-gateway", "food", "console-gateway", "quizcraft")


class Cohorts(unittest.TestCase):
    def test_each_fixed_cohort_selects_only_its_own_containers(self):
        containers = {}
        for service in ("console-gateway", "quizcraft", "portal", "food", "portal-gateway", "nginx"):
            containers[f"henukit-{service}-1"] = {
                "Id": service, "Image": service, "Config": {"Image": service},
                "State": {"Status": "running", "Running": True}, "RestartCount": 0,
            }
        self.assertEqual(
            set(PracticeFixture.snapshot(containers, True)),
            {f"henukit-{s}-1" for s in PracticeFixture.services},
        )
        self.assertEqual(
            set(engine.Release.snapshot(containers, True)),
            {"henukit-food-1", "henukit-portal-gateway-1", "henukit-portal-1"},
        )
        self.assertEqual(
            set(PracticeFixture.snapshot(containers)),
            {"henukit-nginx-1"},
        )


if __name__ == "__main__":
    unittest.main()
