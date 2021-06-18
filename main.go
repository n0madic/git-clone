package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/jessevdk/go-flags"
)

var opts struct {
	Args struct {
		Repository string `positional-arg-name:"repository" required:"yes" description:"The repository to clone from."`
		Directory  string `positional-arg-name:"directory" description:"The name of a new directory to clone into."`
	} `positional-args:"yes"`
	Identity     string `long:"identity" short:"i" value-name:"<file>" description:"Selects a file from which the identity (private key) for public key ssh authentication is read. Or use the environment variable" env:"GIT_CLONE_KEY"`
	Recursive    bool   `long:"recursive" short:"r" description:"After the clone is created, initialize all submodules within, using their default settings."`
	Pull         bool   `long:"pull" short:"p" description:"Incorporates changes from a remote repository into the current branch (if already cloned)."`
	RemoteName   string `long:"origin" short:"o" value-name:"<name>" default:"origin" description:"Instead of using the remote name origin to keep track of the upstream repository, use <name>."`
	Branch       string `long:"branch" short:"b" value-name:"<name>" description:"Instead of pointing the newly created HEAD to the branch pointed to by the cloned repository’s HEAD, point to <name> branch instead." long-description:"If the repository already cloned, it will simply switch the branch, and local changes will be discarded."`
	SingleBranch bool   `long:"single-branch" description:"Clone only the history leading to the tip of a single branch, either specified by the --branch option or the primary branch remote’s HEAD points at."`
	Depth        int    `long:"depth" short:"d" value-name:"<depth>" description:"Create a shallow clone with a history truncated to the specified number of commits."`
	Tags         string `long:"tags" short:"t" description:"Tag mode" default:"all" choice:"all" choice:"no" choice:"following"`
	LastCommit   bool   `long:"last" short:"l" description:"Print the latest commit."`
}

func checkIfError(err error) {
	if err == nil {
		return
	}

	color.Red("%s", err)
	os.Exit(1)
}

func main() {
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(0)
	}

	var Destination string
	if len(opts.Args.Directory) > 0 {
		Destination = opts.Args.Directory
	} else {
		Destination = strings.TrimSuffix(path.Base(opts.Args.Repository), ".git")
	}

	CloneOptions := git.CloneOptions{
		URL:          opts.Args.Repository,
		RemoteName:   opts.RemoteName,
		Depth:        opts.Depth,
		SingleBranch: opts.SingleBranch,
		Progress:     os.Stdout,
	}
	var IdentityKey ssh.AuthMethod
	if len(opts.Identity) > 0 {
		if strings.Contains(opts.Identity, "RSA PRIVATE KEY") {
			IdentityKey, err = ssh.NewPublicKeys("git", []byte(opts.Identity), "")
		} else {
			IdentityKey, err = ssh.NewPublicKeysFromFile("git", opts.Identity, "")
		}
		checkIfError(err)
		CloneOptions.Auth = IdentityKey
	}
	branchRef := plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", opts.Branch))
	if strings.HasPrefix(opts.Branch, "tags/") && opts.Tags != "no" {
		branchRef = plumbing.ReferenceName(fmt.Sprintf("refs/%s", opts.Branch))
	}
	if len(opts.Branch) > 0 {
		CloneOptions.ReferenceName = branchRef
	}
	if opts.Recursive {
		CloneOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}
	switch opts.Tags {
	case "all":
		CloneOptions.Tags = git.AllTags
	case "no":
		CloneOptions.Tags = git.NoTags
	case "following":
		CloneOptions.Tags = git.TagFollowing
	}

	r, err := git.PlainClone(Destination, false, &CloneOptions)

	if err == git.ErrRepositoryAlreadyExists {
		color.Yellow("Repository already exists!")

		r, err := git.PlainOpen(Destination)
		checkIfError(err)

		w, err := r.Worktree()
		checkIfError(err)

		ref, err := r.Head()
		checkIfError(err)

		if len(opts.Branch) > 0 && !strings.HasSuffix(ref.Name().String(), opts.Branch) {
			color.Cyan("Checkout remote branch %s", opts.Branch)
			err = w.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Force:  true,
			})
			if err == plumbing.ErrReferenceNotFound {
				remoteRef, err := r.Reference(plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", opts.RemoteName, opts.Branch)), true)
				checkIfError(err)
				err = w.Checkout(&git.CheckoutOptions{
					Branch: branchRef,
					Hash:   remoteRef.Hash(),
					Create: true,
					Force:  true,
				})
				checkIfError(err)
			} else {
				checkIfError(err)
			}
			ref, err = r.Head()
			checkIfError(err)
		}

		if opts.Pull && ref.Name().IsBranch() {
			color.Cyan("Pull %s", ref.Name())
			PullOptions := git.PullOptions{
				RemoteName:    opts.RemoteName,
				ReferenceName: ref.Name(),
				Depth:         opts.Depth,
				SingleBranch:  opts.SingleBranch,
				Progress:      os.Stdout,
			}
			if len(opts.Identity) > 0 {
				PullOptions.Auth = IdentityKey
			}
			if opts.Recursive {
				PullOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
			}
			err = w.Pull(&PullOptions)
			if err == git.NoErrAlreadyUpToDate {
				color.Green(err.Error())
			} else {
				checkIfError(err)
			}
		}
	} else {
		checkIfError(err)
	}

	if opts.LastCommit {
		fmt.Println()

		if r == nil {
			r, err = git.PlainOpen(Destination)
			checkIfError(err)
		}

		ref, err := r.Head()
		checkIfError(err)
		color.Green("On branch %s", path.Base(ref.Name().String()))

		commit, err := r.CommitObject(ref.Hash())
		checkIfError(err)
		fmt.Print("Show last ")
		fmt.Println(commit)
	}
}
