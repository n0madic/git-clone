package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestHelp(t *testing.T) {
	code, stdout, stderr := runCLI(t, "--help")
	if code != exitUsage {
		t.Fatalf("expected exit %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stdout, "usage: git clone [<options>] [--] <repo> [<dir>]") {
		t.Fatalf("expected usage in stdout, got %q", stdout)
	}
	if strings.Contains(stdout, "--filter") || strings.Contains(stdout, "--reject-shallow") || strings.Contains(stdout, "--bundle-uri") {
		t.Fatalf("expected unsupported flags to be omitted from help, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestUnknownSwitch(t *testing.T) {
	code, _, stderr := runCLI(t, "-r", "repo")
	if code != exitUsage {
		t.Fatalf("expected exit %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stderr, "error: unknown switch `r'") {
		t.Fatalf("expected unknown switch error, got %q", stderr)
	}
}

func TestMissingRepository(t *testing.T) {
	code, _, stderr := runCLI(t)
	if code != exitUsage {
		t.Fatalf("expected exit %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stderr, "fatal: You must specify a repository to clone.") {
		t.Fatalf("expected missing repo error, got %q", stderr)
	}
}

func TestUnsupportedFlag(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")
	code, _, stderr := runCLI(t, "--filter=blob:none", remote, destination)
	if code != exitUsage {
		t.Fatalf("expected exit %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stderr, "error: option `filter' is not supported by this build of git-clone") {
		t.Fatalf("expected unsupported flag error, got %q", stderr)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected destination to remain absent, stat err=%v", err)
	}
}

func TestDepthZeroFails(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")
	code, _, stderr := runCLI(t, "--depth=0", remote, destination)
	if code != exitFatal {
		t.Fatalf("expected exit %d, got %d", exitFatal, code)
	}
	if !strings.Contains(stderr, "fatal: depth 0 is not a positive number") {
		t.Fatalf("expected depth error, got %q", stderr)
	}
}

func TestCloneIntoEmptyExistingDirectory(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runCLI(t, remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}
	assertFileExists(t, filepath.Join(destination, "file.txt"))
}

func TestExistingNonEmptyDirectoryFails(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "existing.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runCLI(t, remote, destination)
	if code != exitFatal {
		t.Fatalf("expected exit %d, got %d", exitFatal, code)
	}
	if !strings.Contains(stderr, "fatal: destination path '") || !strings.Contains(stderr, "already exists and is not an empty directory.") {
		t.Fatalf("expected destination exists error, got %q", stderr)
	}
}

func TestBranchBeatsTagWithSameName(t *testing.T) {
	remote := createBranchTagCollisionRemote(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "-b", "same", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}

	repo, err := git.PlainOpen(destination)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Name() != plumbing.NewBranchReferenceName("same") {
		t.Fatalf("expected branch checkout, got %s", head.Name())
	}
	assertFileExists(t, filepath.Join(destination, "branch.txt"))
}

func TestTagCheckoutDetachedHead(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "-b", "v1.0.0", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}

	repo, err := git.PlainOpen(destination)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Name().IsBranch() {
		t.Fatalf("expected detached head, got branch %s", head.Name())
	}
}

func TestLegacyTagPrefixRejected(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "-b", "tags/v1.0.0", remote, destination)
	if code != exitFatal {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitFatal, code, stderr)
	}
	if !strings.Contains(stderr, "fatal: Remote branch tags/v1.0.0 not found in upstream origin") {
		t.Fatalf("expected missing remote branch error, got %q", stderr)
	}
}

func TestNoTags(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "--no-tags", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}

	repo, err := git.PlainOpen(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Tag("v1.0.0"); err == nil {
		t.Fatal("expected tag v1.0.0 to be absent")
	}
}

func TestNoCheckout(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "--no-checkout", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}
	assertPathAbsent(t, filepath.Join(destination, "file.txt"))
	assertFileExists(t, filepath.Join(destination, ".git", "config"))
}

func TestConfigEntriesWritten(t *testing.T) {
	remote := createBasicRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "-c", "core.filemode=false", "-c", "user.name=FromFlag", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}

	configBytes, err := os.ReadFile(filepath.Join(destination, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configBytes)
	if !strings.Contains(configText, "filemode = false") {
		t.Fatalf("expected core.filemode override in config, got %q", configText)
	}
	if !strings.Contains(configText, "name = FromFlag") {
		t.Fatalf("expected user.name override in config, got %q", configText)
	}
}

func TestPullExtension(t *testing.T) {
	remoteInfo := createBasicRemoteRepoDetails(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, remoteInfo.Remote, destination)
	if code != exitOK {
		t.Fatalf("initial clone failed: code=%d stderr=%q", code, stderr)
	}

	if err := os.WriteFile(filepath.Join(remoteInfo.Source, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, remoteInfo.Source, "git", "add", "new.txt")
	runCmd(t, remoteInfo.Source, "git", "commit", "-m", "new commit")
	runCmd(t, remoteInfo.Source, "git", "push", remoteInfo.Remote, "HEAD:main")

	code, _, stderr = runCLI(t, "--pull", remoteInfo.Remote, destination)
	if code != exitOK {
		t.Fatalf("pull failed: code=%d stderr=%q", code, stderr)
	}
	assertFileExists(t, filepath.Join(destination, "new.txt"))
}

func TestRecursiveClone(t *testing.T) {
	remote := createSubmoduleRemoteRepo(t)
	destination := filepath.Join(t.TempDir(), "clone")

	code, _, stderr := runCLI(t, "--recursive", remote, destination)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d stderr=%q", exitOK, code, stderr)
	}
	assertFileExists(t, filepath.Join(destination, "modules", "submodule.txt"))
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

type remoteInfo struct {
	Source string
	Remote string
}

func createBasicRemoteRepo(t *testing.T) string {
	t.Helper()
	return createBasicRemoteRepoDetails(t).Remote
}

func createBasicRemoteRepoDetails(t *testing.T) remoteInfo {
	t.Helper()

	base := t.TempDir()
	source := filepath.Join(base, "src")
	remote := filepath.Join(base, "remote.git")

	runCmd(t, base, "git", "init", "-b", "main", "src")
	runCmd(t, source, "git", "config", "user.name", "Test User")
	runCmd(t, source, "git", "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, source, "git", "add", "file.txt")
	runCmd(t, source, "git", "commit", "-m", "initial")
	runCmd(t, source, "git", "tag", "v1.0.0")

	runCmd(t, source, "git", "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(source, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, source, "git", "add", "feature.txt")
	runCmd(t, source, "git", "commit", "-m", "feature commit")
	runCmd(t, source, "git", "checkout", "main")

	runCmd(t, base, "git", "clone", "--bare", source, remote)

	return remoteInfo{
		Source: source,
		Remote: remote,
	}
}

func createBranchTagCollisionRemote(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	source := filepath.Join(base, "src")
	remote := filepath.Join(base, "remote.git")

	runCmd(t, base, "git", "init", "-b", "main", "src")
	runCmd(t, source, "git", "config", "user.name", "Test User")
	runCmd(t, source, "git", "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, source, "git", "add", "file.txt")
	runCmd(t, source, "git", "commit", "-m", "base")

	runCmd(t, source, "git", "checkout", "-b", "same")
	if err := os.WriteFile(filepath.Join(source, "branch.txt"), []byte("branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, source, "git", "add", "branch.txt")
	runCmd(t, source, "git", "commit", "-m", "branch")
	runCmd(t, source, "git", "checkout", "main")
	runCmd(t, source, "git", "tag", "same")

	runCmd(t, base, "git", "clone", "--bare", source, remote)
	return remote
}

func createSubmoduleRemoteRepo(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	subSource := filepath.Join(base, "sub-src")
	subRemote := filepath.Join(base, "sub-remote.git")
	mainSource := filepath.Join(base, "main-src")
	mainRemote := filepath.Join(base, "main-remote.git")

	runCmd(t, base, "git", "init", "-b", "main", "sub-src")
	runCmd(t, subSource, "git", "config", "user.name", "Test User")
	runCmd(t, subSource, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(subSource, "submodule.txt"), []byte("submodule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, subSource, "git", "add", "submodule.txt")
	runCmd(t, subSource, "git", "commit", "-m", "submodule init")
	runCmd(t, base, "git", "clone", "--bare", subSource, subRemote)

	runCmd(t, base, "git", "init", "-b", "main", "main-src")
	runCmd(t, mainSource, "git", "config", "user.name", "Test User")
	runCmd(t, mainSource, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(mainSource, "main.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, mainSource, "git", "add", "main.txt")
	runCmd(t, mainSource, "git", "commit", "-m", "main init")
	runCmd(t, mainSource, "git", "-c", "protocol.file.allow=always", "submodule", "add", subRemote, "modules")
	runCmd(t, mainSource, "git", "commit", "-m", "add submodule")
	runCmd(t, base, "git", "clone", "--bare", mainSource, mainRemote)

	return mainRemote
}

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%s", name, strings.Join(args, " "), string(output))
	}

	return string(output)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected %s to be a file", path)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent, err=%v", path, err)
	}
}
