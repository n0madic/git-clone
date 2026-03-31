package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	formatcfg "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/kevinburke/ssh_config"
)

func executeClone(opts cloneOptions, stderr io.Writer) (*git.Repository, error) {
	destination := destinationFor(opts)
	auth, err := buildAuthMethod(opts.Repository, opts.Identity)
	if err != nil {
		return nil, err
	}

	if opts.Pull {
		return cloneOrPull(opts, destination, auth, stderr)
	}

	if err := validateCloneDestination(destination); err != nil {
		return nil, err
	}

	return cloneRepository(opts, destination, auth, stderr)
}

func cloneOrPull(
	opts cloneOptions,
	destination string,
	auth transport.AuthMethod,
	stderr io.Writer,
) (*git.Repository, error) {
	status, err := inspectDestination(destination)
	if err != nil {
		return nil, err
	}

	if !status.exists || status.emptyDir {
		return cloneRepository(opts, destination, auth, stderr)
	}

	repo, err := git.PlainOpen(destination)
	if err != nil {
		return nil, destinationExistsError(destination)
	}

	if opts.Branch != "" {
		targetRef, err := resolveCloneReference(opts.Repository, opts.RemoteName, opts.Branch, auth)
		if err != nil {
			return nil, err
		}
		if targetRef.IsTag() {
			return nil, &cliError{
				code:    exitFatal,
				prefix:  "fatal",
				message: "--pull cannot target a tag",
			}
		}
		if err := checkoutBranch(repo, opts.RemoteName, targetRef); err != nil {
			return nil, err
		}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	if !head.Name().IsBranch() {
		return nil, &cliError{
			code:    exitFatal,
			prefix:  "fatal",
			message: "--pull requires a checked out branch",
		}
	}

	pullOptions := &git.PullOptions{
		RemoteName:    opts.RemoteName,
		RemoteURL:     opts.Repository,
		ReferenceName: head.Name(),
		SingleBranch:  opts.SingleBranch,
		Depth:         opts.Depth,
		Progress:      progressWriter(opts.Progress, stderr),
		Auth:          auth,
	}
	if opts.RecurseSubmodules {
		pullOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}

	err = worktree.Pull(pullOptions)
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil, err
	}

	if err := applyConfigEntries(repo, opts.ConfigEntries); err != nil {
		return nil, err
	}

	return repo, nil
}

func cloneRepository(
	opts cloneOptions,
	destination string,
	auth transport.AuthMethod,
	stderr io.Writer,
) (*git.Repository, error) {
	targetRef, err := resolveCloneReference(opts.Repository, opts.RemoteName, opts.Branch, auth)
	if err != nil {
		return nil, err
	}

	cloneOptions := &git.CloneOptions{
		URL:               opts.Repository,
		RemoteName:        opts.RemoteName,
		ReferenceName:     targetRef,
		SingleBranch:      opts.SingleBranch,
		Mirror:            opts.Mirror,
		NoCheckout:        !opts.Checkout,
		Depth:             opts.Depth,
		RecurseSubmodules: git.NoRecurseSubmodules,
		ShallowSubmodules: opts.ShallowSubmodules,
		Progress:          progressWriter(opts.Progress, stderr),
		Tags:              opts.Tags,
		Auth:              auth,
		Shared:            opts.Shared,
	}
	if opts.RecurseSubmodules {
		cloneOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}

	repo, err := git.PlainClone(destination, opts.Bare, cloneOptions)
	if err != nil {
		return nil, err
	}

	if err := applyConfigEntries(repo, opts.ConfigEntries); err != nil {
		return nil, err
	}

	return repo, nil
}

func resolveCloneReference(repository, remoteName, branch string, auth transport.AuthMethod) (plumbing.ReferenceName, error) {
	if branch == "" {
		return "", nil
	}

	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: remoteName,
		URLs: []string{repository},
	})

	refs, err := remote.List(&git.ListOptions{
		Auth:          auth,
		PeelingOption: git.AppendPeeled,
	})
	if err != nil {
		return "", err
	}

	for _, candidate := range branchCandidates(branch) {
		if hasRemoteReference(refs, candidate) {
			return candidate, nil
		}
	}

	for _, candidate := range tagCandidates(branch) {
		if hasRemoteReference(refs, candidate) {
			return candidate, nil
		}
	}

	return "", &cliError{
		code:    exitFatal,
		prefix:  "fatal",
		message: fmt.Sprintf("Remote branch %s not found in upstream %s", branch, remoteName),
	}
}

func branchCandidates(branch string) []plumbing.ReferenceName {
	if strings.HasPrefix(branch, "refs/heads/") {
		return []plumbing.ReferenceName{plumbing.ReferenceName(branch)}
	}

	if strings.HasPrefix(branch, "refs/") {
		return []plumbing.ReferenceName{plumbing.ReferenceName(branch)}
	}

	return []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName(branch),
	}
}

func tagCandidates(branch string) []plumbing.ReferenceName {
	if strings.HasPrefix(branch, "refs/tags/") {
		return []plumbing.ReferenceName{plumbing.ReferenceName(branch)}
	}

	if strings.HasPrefix(branch, "refs/") {
		return nil
	}

	return []plumbing.ReferenceName{plumbing.NewTagReferenceName(branch)}
}

func hasRemoteReference(refs []*plumbing.Reference, target plumbing.ReferenceName) bool {
	for _, ref := range refs {
		if ref.Name() == target {
			return true
		}
	}

	return false
}

func applyConfigEntries(repo *git.Repository, entries []configEntry) error {
	if len(entries) == 0 {
		return nil
	}

	cfg, err := repo.Config()
	if err != nil {
		return err
	}
	if cfg.Raw == nil {
		cfg.Raw = formatcfg.New()
	}

	for _, entry := range entries {
		cfg.Raw.AddOption(entry.Section, entry.Subsection, entry.Key, entry.Value)
	}

	return repo.SetConfig(cfg)
}

func progressWriter(mode progressMode, stderr io.Writer) io.Writer {
	switch mode {
	case progressForce:
		return stderr
	case progressOff:
		return nil
	case progressAuto:
		if isTerminalWriter(stderr) {
			return stderr
		}
	}

	return nil
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

func buildAuthMethod(repository, identity string) (transport.AuthMethod, error) {
	if identity == "" {
		return nil, nil
	}

	endpoint, err := transport.NewEndpoint(repository)
	if err != nil {
		return nil, err
	}
	if endpoint.Protocol != "ssh" {
		return nil, &cliError{
			code:    exitFatal,
			prefix:  "fatal",
			message: "--identity is only supported for SSH remotes",
		}
	}

	userName, err := sshUserForEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	if looksLikePEM(identity) {
		return gitssh.NewPublicKeys(userName, []byte(identity), "")
	}

	return gitssh.NewPublicKeysFromFile(userName, identity, "")
}

func looksLikePEM(value string) bool {
	return strings.Contains(value, "BEGIN ") && strings.Contains(value, "PRIVATE KEY")
}

func sshUserForEndpoint(endpoint *transport.Endpoint) (string, error) {
	if endpoint.User != "" {
		return endpoint.User, nil
	}

	if endpoint.Host != "" {
		if configured := ssh_config.DefaultUserSettings.Get(endpoint.Host, "User"); configured != "" {
			return configured, nil
		}
	}

	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username, nil
	}

	if envUser := os.Getenv("USER"); envUser != "" {
		return envUser, nil
	}

	return "", &cliError{
		code:    exitFatal,
		prefix:  "fatal",
		message: "failed to determine SSH user for --identity",
	}
}

type destinationStatus struct {
	exists   bool
	emptyDir bool
}

func inspectDestination(destination string) (destinationStatus, error) {
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return destinationStatus{}, nil
		}
		return destinationStatus{}, err
	}

	status := destinationStatus{
		exists: true,
	}
	if !info.IsDir() {
		return status, nil
	}

	entries, err := os.ReadDir(destination)
	if err != nil {
		return destinationStatus{}, err
	}
	status.emptyDir = len(entries) == 0
	return status, nil
}

func validateCloneDestination(destination string) error {
	status, err := inspectDestination(destination)
	if err != nil {
		return err
	}

	if !status.exists || status.emptyDir {
		return nil
	}

	return destinationExistsError(destination)
}

func destinationExistsError(destination string) error {
	return &cliError{
		code:    exitFatal,
		prefix:  "fatal",
		message: fmt.Sprintf("destination path '%s' already exists and is not an empty directory.", destination),
	}
}

func checkoutBranch(repo *git.Repository, remoteName string, branchRef plumbing.ReferenceName) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = worktree.Checkout(&git.CheckoutOptions{Branch: branchRef})
	if err == nil {
		return nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return err
	}

	remoteRef, remoteErr := repo.Reference(
		plumbing.NewRemoteReferenceName(remoteName, branchRef.Short()),
		true,
	)
	if remoteErr != nil {
		return remoteErr
	}

	return worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Hash:   remoteRef.Hash(),
		Create: true,
	})
}

func printLastCommit(repo *git.Repository, stdout io.Writer) error {
	head, err := repo.Head()
	if err != nil {
		return err
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return err
	}

	_, err = io.WriteString(stdout, commit.String())
	return err
}
