//go:build linux

package deploy_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDeployScriptPinsRemoteSHAAndRunsReleasePhases(t *testing.T) {
	fixture := newGitFixture(t)
	sha := fixture.commitAndPush(t, "one.txt", "one")
	paths := fixture.writeConfigAndHook(t, "never")

	result := fixture.runDeploy(t, paths.config, sha, "delivery-1")
	if result.err != nil {
		t.Fatalf("deploy failed: %v\n%s", result.err, result.output)
	}
	phaseLog, err := os.ReadFile(paths.phaseLog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(phaseLog)), "prepare\nactivate\nverify"; got != want {
		t.Fatalf("phases = %q, want %q", got, want)
	}
	deployed, err := os.ReadFile(filepath.Join(paths.stateDir, "deployed-sha"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(deployed)) != sha {
		t.Fatalf("deployed SHA = %q, want %q", deployed, sha)
	}
	currentTarget, err := filepath.EvalSymlinks(paths.currentLink)
	if err != nil {
		t.Fatal(err)
	}
	if currentTarget != filepath.Join(paths.releasesDir, sha) {
		t.Fatalf("current link = %q", currentTarget)
	}
}

func TestDeployScriptIgnoresStaleDelivery(t *testing.T) {
	fixture := newGitFixture(t)
	staleSHA := fixture.commitAndPush(t, "one.txt", "one")
	currentSHA := fixture.commitAndPush(t, "two.txt", "two")
	paths := fixture.writeConfigAndHook(t, "never")

	result := fixture.runDeploy(t, paths.config, staleSHA, "delivery-stale")
	if result.err != nil {
		t.Fatalf("stale deploy returned error: %v\n%s", result.err, result.output)
	}
	normalized := strings.ReplaceAll(result.output, `\ `, " ")
	if !strings.Contains(normalized, "stale delivery ignored") || !strings.Contains(normalized, currentSHA) {
		t.Fatalf("unexpected stale output: %s", result.output)
	}
	if _, err := os.Lstat(paths.currentLink); !os.IsNotExist(err) {
		t.Fatalf("stale delivery changed current link: %v", err)
	}
}

func TestDeployScriptRejectsUnexpectedRemoteURL(t *testing.T) {
	fixture := newGitFixture(t)
	sha := fixture.commitAndPush(t, "one.txt", "one")
	paths := fixture.writeConfigAndHook(t, "never")
	config, err := os.ReadFile(paths.config)
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.Replace(string(config), fixture.remote, filepath.Join(fixture.root, "unexpected.git"), 1)
	if err := os.WriteFile(paths.config, []byte(wrong), 0o640); err != nil {
		t.Fatal(err)
	}
	result := fixture.runDeploy(t, paths.config, sha, "delivery-remote")
	if result.err == nil || !strings.Contains(strings.ReplaceAll(result.output, `\ `, " "), "remote URL does not match") {
		t.Fatalf("unexpected remote was accepted: %v\n%s", result.err, result.output)
	}
}

func TestDeployScriptStagesHighRiskReleaseUntilApproved(t *testing.T) {
	fixture := newGitFixture(t)
	sha := fixture.commitAndPush(t, "services/platform-core/change.txt", "high risk")
	paths := fixture.writeConfigAndHook(t, "high-risk")

	result := fixture.runDeploy(t, paths.config, sha, "delivery-gated")
	if result.err == nil {
		t.Fatal("high-risk first release unexpectedly activated")
	}
	var exitError *exec.ExitError
	normalized := strings.ReplaceAll(result.output, `\ `, " ")
	if !strings.Contains(normalized, "manual approval is required") || !asExitError(result.err, &exitError) || exitError.ExitCode() != 78 {
		t.Fatalf("unexpected gate result: %v\n%s", result.err, result.output)
	}
	approval := filepath.Join(paths.approvalDir, sha)
	if err := os.WriteFile(approval, []byte(sha+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	result = fixture.runDeploy(t, paths.config, sha, "delivery-approved")
	if result.err != nil {
		t.Fatalf("approved deploy failed: %v\n%s", result.err, result.output)
	}
}

type gitFixture struct {
	root       string
	remote     string
	source     string
	repository string
	script     string
}

type deployPaths struct {
	config      string
	phaseLog    string
	stateDir    string
	approvalDir string
	releasesDir string
	currentLink string
}

type commandResult struct {
	output string
	err    error
}

func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	repository := filepath.Join(root, "repository")
	run(t, "", "git", "init", "--bare", remote)
	run(t, "", "git", "init", "-b", "main", source)
	run(t, source, "git", "config", "user.name", "Webhook Test")
	run(t, source, "git", "config", "user.email", "webhook@example.invalid")
	run(t, source, "git", "remote", "add", "origin", remote)
	serviceDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return &gitFixture{root: root, remote: remote, source: source, repository: repository, script: filepath.Join(serviceDir, "deploy", "henukit-deploy")}
}

func (f *gitFixture) commitAndPush(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(f.source, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, f.source, "git", "add", ".")
	run(t, f.source, "git", "commit", "-m", "test commit")
	run(t, f.source, "git", "push", "-u", "origin", "main")
	if _, err := os.Stat(filepath.Join(f.repository, ".git")); os.IsNotExist(err) {
		run(t, "", "git", "clone", "--branch", "main", f.remote, f.repository)
	}
	return strings.TrimSpace(run(t, f.source, "git", "rev-parse", "HEAD"))
}

func (f *gitFixture) writeConfigAndHook(t *testing.T, approvalMode string) deployPaths {
	t.Helper()
	releasesDir := filepath.Join(f.root, "releases")
	stateDir := filepath.Join(f.root, "state")
	approvalDir := filepath.Join(f.root, "approvals")
	hookDir := filepath.Join(f.root, "hooks")
	currentLink := filepath.Join(f.root, "current")
	phaseLog := filepath.Join(f.root, "phases.log")
	for _, dir := range []string{releasesDir, stateDir, approvalDir, hookDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(f.root, "deploy.env")
	configBody := fmt.Sprintf(`HENUKIT_ALLOWED_REPOSITORY=jry21223/HENU-Kit-DEV
HENUKIT_GIT_BRANCH=main
HENUKIT_GIT_REMOTE=origin
HENUKIT_EXPECTED_REMOTE_URL=%s
HENUKIT_REPO_DIR=%s
HENUKIT_RELEASES_DIR=%s
HENUKIT_CURRENT_LINK=%s
HENUKIT_DEPLOY_HOOK_DIR=%s
HENUKIT_DEPLOY_STATE_DIR=%s
HENUKIT_APPROVAL_DIR=%s
HENUKIT_KEEP_RELEASES=3
HENUKIT_REQUIRE_APPROVAL=%s
HENUKIT_ALLOW_FIRST_AUTO_DEPLOY=0
`, f.remote, f.repository, releasesDir, currentLink, hookDir, stateDir, approvalDir, approvalMode)
	if err := os.WriteFile(config, []byte(configBody), 0o640); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hookDir, "10-test")
	hookBody := fmt.Sprintf("#!/bin/sh\nset -eu\ncase \"$1\" in rollback) exit 0;; *) printf '%%s\\n' \"$1\" >> %q;; esac\n", phaseLog)
	if err := os.WriteFile(hook, []byte(hookBody), 0o700); err != nil {
		t.Fatal(err)
	}
	return deployPaths{config: config, phaseLog: phaseLog, stateDir: stateDir, approvalDir: approvalDir, releasesDir: releasesDir, currentLink: currentLink}
}

func (f *gitFixture) runDeploy(t *testing.T, config, sha, delivery string) commandResult {
	t.Helper()
	uid := strconv.Itoa(os.Getuid())
	command := exec.Command(f.script,
		"--sha", sha,
		"--delivery", delivery,
		"--repository", "jry21223/HENU-Kit-DEV",
		"--ref", "refs/heads/main",
	)
	command.Env = append(os.Environ(),
		"HENUKIT_DEPLOY_CONFIG="+config,
		"HENUKIT_CONFIG_OWNER_UID="+uid,
		"HENUKIT_HOOK_OWNER_UID="+uid,
		"HENUKIT_APPROVAL_OWNER_UID="+uid,
		"HENUKIT_DEPLOY_LOCK_FILE="+filepath.Join(f.root, "deploy.lock"),
	)
	output, err := command.CombinedOutput()
	return commandResult{output: string(output), err: err}
}

func run(t *testing.T, dir, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, arguments, err, output)
	}
	return string(output)
}

func asExitError(err error, target **exec.ExitError) bool {
	exitError, ok := err.(*exec.ExitError)
	if ok {
		*target = exitError
	}
	return ok
}
