import importlib.util
import hashlib
import io
import json
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock


MODULE_PATH = pathlib.Path(__file__).with_name("verify_node.py")
SPEC = importlib.util.spec_from_file_location("verify_node", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
verify_node = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(verify_node)


class HealthyNodeProbe:
    def current_main_sha(self):
        return "a" * 40

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
                "HENUKIT_GETWORK_PROVENANCE_MODE=ssh-signature\n"
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

    def image_matches_archive(self, image, path):
        return image == f"henukit-getwork-mcp:{'a' * 40}"

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


class StaleMainProbe(HealthyNodeProbe):
    def current_main_sha(self):
        return "f" * 40


class HealthyActionsProbe(HealthyNodeProbe):
    def secure_file(self, path):
        main_ref_contents = (
            "format=henukit-current-main-ref-v1\n"
            "source_repository=jry21223/HENU-Kit-DEV\n"
            "source_ref=refs/heads/main\n"
            f"release_sha={'a' * 40}\n"
        )
        if path == pathlib.Path("/etc/henukit-getwork/node.env"):
            item = super().secure_file(path)
            return item._replace(
                contents=item.contents.replace(
                    "HENUKIT_GETWORK_PROVENANCE_MODE=ssh-signature\n",
                    "HENUKIT_GETWORK_PROVENANCE_MODE=github-actions\n",
                )
            )
        if path.name.endswith(".manifest"):
            return verify_node.SecureFile(
                True,
                False,
                0,
                0o400,
                "format=henukit-getwork-actions-release-v1\n"
                f"release_sha={'a' * 40}\n"
                "source_repository=jry21223/HENU-Kit-DEV\n"
                "source_ref=refs/heads/main\n"
                "signer_workflow=.github/workflows/deploy-henukit.yml\n"
                "builder_platform=linux/amd64\n"
                f"artifact_sha256={'c' * 64}  henukit-getwork-mcp-{'a' * 40}.docker.tar.gz\n",
            )
        if path.name.endswith(".attestation.json"):
            return verify_node.SecureFile(True, False, 0, 0o400, "attestation")
        if path.name == "trusted_root.jsonl":
            return verify_node.SecureFile(True, False, 0, 0o400, "trusted-root")
        if path.name == "main-ref.env":
            return verify_node.SecureFile(
                True,
                False,
                0,
                0o400,
                main_ref_contents,
            )
        if path == pathlib.Path("/etc/henukit-getwork/trust.env"):
            item = super().secure_file(path)
            return item._replace(
                contents=item.contents
                + "HENUKIT_GETWORK_SIGSTORE_TRUSTED_ROOT_SHA256="
                + hashlib.sha256(b"trusted-root").hexdigest()
                + "\nHENUKIT_GETWORK_CURRENT_MAIN_REF_SHA256="
                + hashlib.sha256(main_ref_contents.encode()).hexdigest()
                + "\n"
            )
        if path == pathlib.Path("/usr/bin/gh"):
            return verify_node.SecureFile(True, False, 0, 0o755, "github-cli")
        return super().secure_file(path)

    def actions_attestation_valid(self, manifest, attestation, gh_file, release_sha, custom_trusted_root):
        self.actions_attestation = (manifest, attestation, gh_file, release_sha, custom_trusted_root)
        return True


class TamperedActionsTrustedRootProbe(HealthyActionsProbe):
    def secure_file(self, path):
        if path.name == "trusted_root.jsonl":
            return verify_node.SecureFile(True, False, 0, 0o400, "tampered-root")
        return super().secure_file(path)


class VerifyNodeTests(unittest.TestCase):
    def test_historical_main_release_is_rejected(self):
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

        with self.assertRaisesRegex(verify_node.VerificationError, "current origin/main"):
            verify_node.verify(config, StaleMainProbe())

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

    def test_healthy_actions_node_reverifies_the_exact_main_attestation(self):
        probe = HealthyActionsProbe()
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
            manifest_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-getwork-actions-{'a' * 40}.manifest"),
            signature_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-release-{'a' * 40}.manifest.sig"),
            allowed_signers_file=pathlib.Path("/etc/henukit-getwork/release-signers"),
            provenance_mode="github-actions",
            attestation_file=pathlib.Path(f"/var/lib/henukit-getwork-artifacts/henukit-getwork-actions-{'a' * 40}.attestation.json"),
            gh_file=pathlib.Path("/usr/bin/gh"),
            actions_custom_trusted_root_file=pathlib.Path("/etc/henukit-getwork/trusted_root.jsonl"),
            current_main_ref_file=pathlib.Path("/etc/henukit-getwork/main-ref.env"),
        )

        evidence = verify_node.verify(config, probe)

        self.assertEqual(evidence.source_count, 18)
        self.assertEqual(probe.actions_attestation[-2], "a" * 40)
        self.assertEqual(
            probe.actions_attestation[-1],
            pathlib.Path("/etc/henukit-getwork/trusted_root.jsonl"),
        )

        with self.assertRaisesRegex(verify_node.VerificationError, "custom trusted root path"):
            verify_node.verify(
                config._replace(actions_custom_trusted_root_file=None),
                HealthyActionsProbe(),
            )
        with self.assertRaisesRegex(verify_node.VerificationError, "trusted-root digest"):
            verify_node.verify(config, TamperedActionsTrustedRootProbe())

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

    def test_real_probe_binds_oci_manifest_config_and_rootfs_layers(self):
        release_sha = "a" * 40
        image = f"henukit-getwork-mcp:{release_sha}"
        layer_contents = b"fixture-layer\n"
        layer_digest = hashlib.sha256(layer_contents).hexdigest()
        layer_id = f"sha256:{layer_digest}"
        image_config = {
            "architecture": "amd64",
            "os": "linux",
            "config": {"User": "65532:65532", "Env": ["FIXTURE=1"]},
            "rootfs": {"type": "layers", "diff_ids": [layer_id]},
        }
        config_contents = json.dumps(image_config, separators=(",", ":")).encode()
        config_digest = hashlib.sha256(config_contents).hexdigest()

        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            config_name = f"blobs/sha256/{config_digest}"
            layer_name = f"blobs/sha256/{layer_digest}"
            (root / "blobs" / "sha256").mkdir(parents=True)
            (root / config_name).write_bytes(config_contents)
            (root / layer_name).write_bytes(layer_contents)
            (root / "manifest.json").write_text(
                json.dumps([{
                    "Config": config_name,
                    "RepoTags": [image],
                    "Layers": [layer_name],
                }]),
                encoding="utf-8",
            )
            archive_path = root / "image.docker.tar.gz"
            with tarfile.open(archive_path, mode="w:gz") as archive:
                archive.add(root / "manifest.json", arcname="manifest.json")
                archive.add(root / config_name, arcname=config_name)
                archive.add(root / layer_name, arcname=layer_name)

            probe = verify_node.RealProbe()

            def docker_inspect(*args):
                if "{{json .Config}}" in args:
                    return '{"Env":["FIXTURE=1"],"User":"65532:65532"}'
                if "{{json .RootFS.Layers}}" in args:
                    return json.dumps([layer_id])
                raise AssertionError(args)

            with mock.patch.object(probe, "_command", side_effect=docker_inspect):
                self.assertTrue(probe.image_matches_archive(image, archive_path))

            different_layer_contents = b"different-fixture-layer\n"
            different_layer_digest = hashlib.sha256(different_layer_contents).hexdigest()
            different_layer_name = f"blobs/sha256/{different_layer_digest}"
            (root / different_layer_name).write_bytes(different_layer_contents)
            (root / "manifest.json").write_text(
                json.dumps([{
                    "Config": config_name,
                    "RepoTags": [image],
                    "Layers": [different_layer_name],
                }]),
                encoding="utf-8",
            )
            mismatched_archive_path = root / "mismatched-image.docker.tar.gz"
            with tarfile.open(mismatched_archive_path, mode="w:gz") as archive:
                archive.add(root / "manifest.json", arcname="manifest.json")
                archive.add(root / config_name, arcname=config_name)
                archive.add(root / different_layer_name, arcname=different_layer_name)

            with mock.patch.object(probe, "_command", side_effect=docker_inspect):
                self.assertFalse(
                    probe.image_matches_archive(image, mismatched_archive_path)
                )

    def test_actions_verification_does_not_inherit_github_tokens(self):
        completed = subprocess.CompletedProcess([], 0, stdout="[{}]", stderr="")
        with mock.patch.dict(
            os.environ,
            {"GH_TOKEN": "must-not-leak", "GITHUB_TOKEN": "must-not-leak"},
        ), mock.patch.object(verify_node.subprocess, "run", return_value=completed) as run:
            valid = verify_node.RealProbe().actions_attestation_valid(
                pathlib.Path("/release.manifest"),
                pathlib.Path("/release.attestation.json"),
                pathlib.Path("/usr/bin/gh"),
                "a" * 40,
                pathlib.Path("/etc/henukit-getwork/trusted_root.jsonl"),
            )

        self.assertTrue(valid)
        environment = run.call_args.kwargs["env"]
        self.assertNotIn("GH_TOKEN", environment)
        self.assertNotIn("GITHUB_TOKEN", environment)
        self.assertEqual(environment["GH_PROMPT_DISABLED"], "1")
        self.assertIn(
            verify_node.ACTIONS_PREDICATE_TYPE,
            run.call_args.args[0],
        )
        self.assertIn(
            "--custom-trusted-root",
            run.call_args.args[0],
        )
        self.assertIn(
            "/etc/henukit-getwork/trusted_root.jsonl",
            run.call_args.args[0],
        )

    def test_current_main_lookup_ignores_inherited_git_rewrites(self):
        completed = subprocess.CompletedProcess(
            [], 0, stdout=f"{'a' * 40}\trefs/heads/main\n", stderr=""
        )
        injected = {
            "GIT_CONFIG_COUNT": "1",
            "GIT_CONFIG_KEY_0": "url.file:///tmp/attacker/.insteadOf",
            "GIT_CONFIG_VALUE_0": "https://github.com/jry21223/HENU-Kit-DEV.git",
            "GIT_EXEC_PATH": "/tmp/attacker",
        }
        with mock.patch.dict(os.environ, injected), mock.patch.object(
            verify_node.subprocess, "run", return_value=completed
        ) as run:
            current = verify_node.RealProbe().current_main_sha()

        self.assertEqual(current, "a" * 40)
        environment = run.call_args.kwargs["env"]
        self.assertTrue(set(injected).isdisjoint(environment))
        self.assertEqual(run.call_args.args[0][0], "/usr/bin/git")
        self.assertEqual(run.call_args.kwargs["cwd"], "/")
        self.assertIn("protocol.allow=never", run.call_args.args[0])

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

    def test_mismatched_live_container_or_archive_image_fails_closed(self):
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
        for method in ("runtime_hardened", "image_matches_archive"):
            probe = HealthyNodeProbe()
            setattr(probe, method, lambda *_: False)
            with self.subTest(method=method):
                with self.assertRaises(verify_node.VerificationError):
                    verify_node.verify(config, probe)


if __name__ == "__main__":
    unittest.main()
