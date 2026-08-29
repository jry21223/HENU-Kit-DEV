import importlib.util
import hashlib
import io
import pathlib
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("verify_node.py")
SPEC = importlib.util.spec_from_file_location("verify_node", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
verify_node = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(verify_node)


class HealthyNodeProbe:
    def osrelease(self):
        return "6.6.87.2-microsoft-standard-WSL2"

    def machine(self):
        return "x86_64"

    def root_fstype(self):
        return "ext4"

    def docker_platform(self, image):
        self.image = image
        return "linux/amd64"

    def docker_image_id(self, image):
        return "sha256:" + "b" * 64

    def trusted_parent_chain(self, path):
        return True

    def account_uid(self, name):
        return 998

    def account_contract(self, name):
        return True

    def secure_file(self, path):
        if path.name.endswith(".docker.tar.gz"):
            return verify_node.SecureFile(True, False, 0, 0o400, "")
        if path.name == "henukit-getwork-egress":
            return verify_node.SecureFile(True, False, 0, 0o755, "egress-helper")
        if path.name == "verify_node.py":
            return verify_node.SecureFile(True, False, 0, 0o755, "verifier")
        if path.parent.name == "systemd" and path.name == "henukit-getwork-mcp.service":
            return verify_node.SecureFile(True, False, 0, 0o644, "mcp-unit")
        if path.parent.name == "systemd" and path.name == "henukit-getwork-tunnel.service":
            return verify_node.SecureFile(True, False, 0, 0o644, "tunnel-unit")
        if path.name == "trust.env":
            return verify_node.SecureFile(
                True,
                False,
                0,
                0o600,
                "HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:tunnel\n"
                "HENUKIT_GETWORK_HOST_KEY_FINGERPRINT=SHA256:host\n",
            )
        if path.name.endswith(".manifest"):
            return verify_node.SecureFile(
                True,
                False,
                0,
                0o400,
                "format=henukit-local-release-v1\n"
                f"release_sha={'a' * 40}\n"
                "source_ref=refs/heads/main\n"
                "builder_platform=linux/amd64\n"
                "signer=henukit-release\n"
                "signature_namespace=henukit-release\n"
                f"artifact_sha256={'c' * 64}  henukit-getwork-mcp-{'a' * 40}.docker.tar.gz\n",
            )
        if path.name.endswith(".manifest.sig"):
            return verify_node.SecureFile(True, False, 0, 0o400, "signature")
        if path.name == "release-signers":
            return verify_node.SecureFile(True, False, 0, 0o644, "signers")
        contents = {
            pathlib.Path("/etc/henukit-getwork/node.env"): (
                f"HENUKIT_GETWORK_RELEASE_SHA={'a' * 40}\n"
                f"HENUKIT_GETWORK_IMAGE_ID=sha256:{'b' * 64}\n"
                "HENUKIT_GETWORK_ARCHIVE_SHA256=" + "c" * 64 + "\n"
                + "HENUKIT_GETWORK_MCP_UNIT_SHA256="
                + hashlib.sha256(b"mcp-unit").hexdigest()
                + "\nHENUKIT_GETWORK_TUNNEL_UNIT_SHA256="
                + hashlib.sha256(b"tunnel-unit").hexdigest()
                + "\nHENUKIT_GETWORK_EGRESS_SHA256="
                + hashlib.sha256(b"egress-helper").hexdigest()
                + "\n"
                "HENUKIT_GETWORK_MEMORY_LIMIT=4g\n"
                "HENUKIT_GETWORK_TUNNEL_TARGET=henukit-getwork-tunnel@8.146.200.82\n"
                "HENUKIT_GETWORK_TUNNEL_PORT=22222\n"
            ),
            pathlib.Path("/etc/henukit-getwork/mcp.env"): (
                "GETWORK_MCP_ACCESS_TOKEN=deployment-owned-getwork-token-0000000000000000\n"
            ),
            pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"): "private-key-not-returned",
            pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"): "approved-host-key",
            pathlib.Path("/etc/systemd/system/henukit-getwork-mcp.service"): "mcp-unit",
            pathlib.Path("/etc/systemd/system/henukit-getwork-tunnel.service"): "tunnel-unit",
        }
        modes = {
            pathlib.Path("/etc/henukit-getwork/node.env"): 0o644,
            pathlib.Path("/etc/henukit-getwork/mcp.env"): 0o600,
            pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"): 0o600,
            pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"): 0o640,
            pathlib.Path("/etc/systemd/system/henukit-getwork-mcp.service"): 0o644,
            pathlib.Path("/etc/systemd/system/henukit-getwork-tunnel.service"): 0o644,
        }
        return verify_node.SecureFile(True, False, 0, modes[path], contents[path])

    def unit_matches_source(self, name, source_dir, installed_dir):
        return True

    def private_key_fingerprint(self, path):
        return "SHA256:tunnel"

    def known_host_fingerprints(self, path, host, port):
        return ["SHA256:host"]

    def signed_manifest_valid(self, manifest, signature, allowed_signers):
        return True

    def archive_sha256(self, path):
        return "c" * 64

    def archive_image_id(self, path):
        return "sha256:" + "b" * 64

    def runtime_hardened(self, expected_image_id):
        return expected_image_id == "sha256:" + "b" * 64

    def egress_policy_live(self):
        return True

    def service_active(self, name):
        return name in {"henukit-getwork-mcp.service", "henukit-getwork-tunnel.service"}

    def health(self):
        return {"ok": True, "upstream": "RyaoVen/getWork@2c7800d"}

    def tools(self, token):
        self.tool_token = token
        return ["list_sources", "crawl_jobs"]

    def sources(self, token):
        self.source_token = token
        return [
            "alibaba",
            "baidu",
            "beike",
            "bytedance",
            "ctrip",
            "dewu",
            "didi",
            "jd",
            "kuaishou",
            "meituan",
            "netease",
            "pdd",
            "tencent",
            "tencentmusic",
            "tongcheng",
            "vipshop",
            "xfusion",
            "xiaohongshu",
        ]

    def crawl(self, token, source):
        self.crawl_source = source
        return {"status": "ok", "source": source, "jobs": []}


class WrongSourcesProbe(HealthyNodeProbe):
    def sources(self, token):
        return [f"source-{index}" for index in range(18)]


class VerifyNodeTests(unittest.TestCase):
    def test_rpc_envelope_rejects_wrong_id_error_and_malformed_shapes(self):
        valid = {"jsonrpc": "2.0", "id": 3, "result": {"content": []}}
        self.assertEqual(verify_node._rpc_result(valid, 3), {"content": []})
        for envelope in (
            {"jsonrpc": "2.0", "id": 999, "result": {}},
            {"jsonrpc": "2.0", "id": 3, "error": {"code": -1}, "result": {}},
            {"jsonrpc": "1.0", "id": 3, "result": {}},
            {"jsonrpc": "2.0", "id": 3, "result": []},
            ["not", "an", "object"],
        ):
            with self.subTest(envelope=envelope):
                with self.assertRaises(verify_node.VerificationError):
                    verify_node._rpc_result(envelope, 3)

    def test_tool_payload_rejects_is_error_and_non_object_text(self):
        for result in (
            {"isError": True, "content": [{"type": "text", "text": '{"status":"ok"}'}]},
            {"isError": 1, "content": [{"type": "text", "text": '{"status":"ok"}'}]},
            {"isError": "true", "content": [{"type": "text", "text": '{"status":"ok"}'}]},
            {"isError": [], "content": [{"type": "text", "text": '{"status":"ok"}'}]},
            {"isError": {}, "content": [{"type": "text", "text": '{"status":"ok"}'}]},
            {"content": "not-a-list"},
            {"content": [{"type": "text", "text": "[]"}]},
            {"content": [{"type": "text", "text": 42}]},
        ):
            with self.subTest(result=result):
                with self.assertRaises((verify_node.VerificationError, ValueError)):
                    verify_node._tool_payload(result, "crawl_jobs")

    def test_bounded_json_rejects_oversize_expired_and_non_object_responses(self):
        with self.assertRaises(verify_node.VerificationError):
            verify_node._read_bounded_json(
                io.BytesIO(b'{"payload":"' + b"x" * 64 + b'"}'),
                32,
                float("inf"),
            )
        with self.assertRaises(verify_node.VerificationError):
            verify_node._read_bounded_json(io.BytesIO(b"{}"), 32, 0)
        with self.assertRaises(verify_node.VerificationError):
            verify_node._read_bounded_json(io.BytesIO(b"[]"), 32, float("inf"))

    def test_healthy_node_proves_the_complete_private_crawler_contract(self):
        probe = HealthyNodeProbe()
        config = verify_node.Config(
            release_sha="a" * 40,
            token_file=pathlib.Path("/etc/henukit-getwork/mcp.env"),
            node_env_file=pathlib.Path("/etc/henukit-getwork/node.env"),
            private_key_file=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"),
            known_hosts_file=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"),
            artifact_file=pathlib.Path(f"/var/lib/henukit-getwork-tunnel/artifacts/henukit-getwork-mcp-{'a' * 40}.docker.tar.gz"),
            source_unit_dir=pathlib.Path("/checkout/services/getwork-mcp/deploy/systemd"),
            installed_unit_dir=pathlib.Path("/etc/systemd/system"),
            installed_egress_file=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"),
            trust_file=pathlib.Path("/etc/henukit-getwork/trust.env"),
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
        )

        evidence = verify_node.verify(config, probe)

        self.assertEqual(evidence.image, f"henukit-getwork-mcp:{'a' * 40}")
        self.assertEqual(evidence.source_count, 18)
        self.assertEqual(evidence.tools, ("crawl_jobs", "list_sources"))
        self.assertEqual(evidence.crawl_source, "alibaba")
        self.assertNotIn("deployment-owned-getwork-token", repr(evidence))

    def test_eighteen_unapproved_source_keys_do_not_pass_as_the_pinned_set(self):
        config = verify_node.Config(
            release_sha="a" * 40,
            token_file=pathlib.Path("/etc/henukit-getwork/mcp.env"),
            node_env_file=pathlib.Path("/etc/henukit-getwork/node.env"),
            private_key_file=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"),
            known_hosts_file=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"),
            artifact_file=pathlib.Path(f"/var/lib/henukit-getwork-tunnel/artifacts/henukit-getwork-mcp-{'a' * 40}.docker.tar.gz"),
            source_unit_dir=pathlib.Path("/checkout/services/getwork-mcp/deploy/systemd"),
            installed_unit_dir=pathlib.Path("/etc/systemd/system"),
            installed_egress_file=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"),
            trust_file=pathlib.Path("/etc/henukit-getwork/trust.env"),
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
        )

        with self.assertRaises(verify_node.VerificationError):
            verify_node.verify(config, WrongSourcesProbe())

    def test_real_probe_does_not_follow_a_secret_symlink(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            target = root / "target.env"
            target.write_text("GETWORK_MCP_ACCESS_TOKEN=must-not-be-read\n", encoding="utf-8")
            link = root / "mcp.env"
            link.symlink_to(target)

            token_file = verify_node.RealProbe().secure_file(link)

            self.assertTrue(token_file.symlink)
            self.assertEqual(token_file.contents, "")

    def test_tampered_installed_unit_fails_closed(self):
        probe = HealthyNodeProbe()
        probe.unit_matches_source = lambda *_: False
        config = verify_node.Config(
            release_sha="a" * 40,
            token_file=pathlib.Path("/etc/henukit-getwork/mcp.env"),
            node_env_file=pathlib.Path("/etc/henukit-getwork/node.env"),
            private_key_file=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"),
            known_hosts_file=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"),
            artifact_file=pathlib.Path(f"/var/lib/henukit-getwork-tunnel/artifacts/henukit-getwork-mcp-{'a' * 40}.docker.tar.gz"),
            source_unit_dir=pathlib.Path("/checkout/services/getwork-mcp/deploy/systemd"),
            installed_unit_dir=pathlib.Path("/etc/systemd/system"),
            installed_egress_file=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"),
            trust_file=pathlib.Path("/etc/henukit-getwork/trust.env"),
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
        )
        with self.assertRaises(verify_node.VerificationError):
            verify_node.verify(config, probe)

    def test_unsigned_manifest_and_inactive_egress_fail_closed(self):
        config = verify_node.Config(
            release_sha="a" * 40,
            token_file=pathlib.Path("/etc/henukit-getwork/mcp.env"),
            node_env_file=pathlib.Path("/etc/henukit-getwork/node.env"),
            private_key_file=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"),
            known_hosts_file=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"),
            artifact_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-getwork-mcp-{'a' * 40}.docker.tar.gz"),
            source_unit_dir=pathlib.Path("/checkout/services/getwork-mcp/deploy/systemd"),
            installed_unit_dir=pathlib.Path("/etc/systemd/system"),
            installed_egress_file=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"),
            trust_file=pathlib.Path("/etc/henukit-getwork/trust.env"),
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
        )
        for method in ("signed_manifest_valid", "egress_policy_live", "account_contract"):
            probe = HealthyNodeProbe()
            setattr(probe, method, lambda *_: False)
            with self.subTest(method=method):
                with self.assertRaises(verify_node.VerificationError):
                    verify_node.verify(config, probe)

    def test_mismatched_live_container_image_fails_closed(self):
        probe = HealthyNodeProbe()
        probe.runtime_hardened = lambda *_: False
        config = verify_node.Config(
            release_sha="a" * 40,
            token_file=pathlib.Path("/etc/henukit-getwork/mcp.env"),
            node_env_file=pathlib.Path("/etc/henukit-getwork/node.env"),
            private_key_file=pathlib.Path("/etc/henukit-getwork/tunnel/id_ed25519"),
            known_hosts_file=pathlib.Path("/etc/henukit-getwork/tunnel/known_hosts"),
            artifact_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-getwork-mcp-{'a' * 40}.docker.tar.gz"),
            source_unit_dir=pathlib.Path("/checkout/services/getwork-mcp/deploy/systemd"),
            installed_unit_dir=pathlib.Path("/etc/systemd/system"),
            installed_egress_file=pathlib.Path("/usr/local/libexec/henukit-getwork-egress"),
            trust_file=pathlib.Path("/etc/henukit-getwork/trust.env"),
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
        )
        with self.assertRaises(verify_node.VerificationError):
            verify_node.verify(config, probe)


if __name__ == "__main__":
    unittest.main()
