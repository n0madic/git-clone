package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/fatih/color"
)

var (
	Repository = kingpin.Arg("repository", "The repository to clone from.").Required().String()
	Directory  = kingpin.Arg("directory", "The name of a new directory to clone into.").String()
	Identity   = kingpin.Flag("identity", "Selects a file from which the identity (private key) for public key ssh authentication is read.").Short('i').PlaceHolder("<file>").String()
	Recursive  = kingpin.Flag("recursive", "After the clone is created, initialize all submodules within, using their default settings.").Short('r').Bool()
	Pull       = kingpin.Flag("pull", "Incorporates changes from a remote repository into the current branch (if already cloned).").Short('p').Bool()
	RemoteName = kingpin.Flag("origin", "Instead of using the remote name origin to keep track of the upstream repository, use <name>.").Short('o').PlaceHolder("<name>").Default("origin").String()
	Branch     = kingpin.Flag("branch", "Instead of pointing the newly created HEAD to the branch pointed to by the cloned repository’s HEAD, point to <name> branch instead. "+
		"If the repository already cloned, it will simply switch the branch, and local changes will be discarded.").Short('b').PlaceHolder("<name>").String()
	SingleBranch = kingpin.Flag("single-branch", "Clone only the history leading to the tip of a single branch, either specified by the --branch option or the primary branch remote’s HEAD points at.").Bool()
	Depth        = kingpin.Flag("depth", "Create a shallow clone with a history truncated to the specified number of commits.").Short('d').PlaceHolder("<depth>").Int()
	Tags         = kingpin.Flag("tags", "Tag mode (all|no|following)").Default("all").Enum("all", "no", "following")
	LastCommit   = kingpin.Flag("last", "Print the latest commit.").Short('l').Bool()
)

func CheckIfError(err error) {
	if err == nil {
		return
	}

	color.Red("%s", err)
	os.Exit(1)
}

func main() {
	kingpin.Parse()

	var Destination string
	if len(*Directory) > 0 {
		Destination = *Directory
	} else {
		Destination = strings.TrimSuffix(path.Base(*Repository), ".git")
	}

	CloneOptions := git.CloneOptions{
		URL:          *Repository,
		RemoteName:   *RemoteName,
		Depth:        *Depth,
		SingleBranch: *SingleBranch,
		Progress:     os.Stdout,
	}
	if len(*Identity) > 0 {
		auth, err := ssh.NewPublicKeysFromFile("git", *Identity, "")
		CheckIfError(err)
		CloneOptions.Auth = auth
	}
	branchRef := plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", *Branch))
	if strings.HasPrefix(*Branch, "tags/") && *Tags != "no" {
		branchRef = plumbing.ReferenceName(fmt.Sprintf("refs/%s", *Branch))
	}
	if len(*Branch) > 0 {
		CloneOptions.ReferenceName = branchRef
	}
	if *Recursive {
		CloneOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}
	switch *Tags {
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
		CheckIfError(err)

		w, err := r.Worktree()
		CheckIfError(err)

		ref, err := r.Head()
		CheckIfError(err)

		if len(*Branch) > 0 && !strings.HasSuffix(ref.Name().String(), *Branch) {
			color.Cyan("Checkout remote branch %s", *Branch)
			err = w.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Force:  true,
			})
			if err == plumbing.ErrReferenceNotFound {
				remoteRef, err := r.Reference(plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", *RemoteName, *Branch)), true)
				CheckIfError(err)
				err = w.Checkout(&git.CheckoutOptions{
					Branch: branchRef,
					Hash:   remoteRef.Hash(),
					Create: true,
					Force:  true,
				})
			} else {
				CheckIfError(err)
			}
			ref, err = r.Head()
			CheckIfError(err)
		}

		if *Pull && ref.Name().IsBranch() {
			color.Cyan("Pull %s", ref.Name())
			PullOptions := git.PullOptions{
				RemoteName:    *RemoteName,
				ReferenceName: ref.Name(),
				Depth:         *Depth,
				SingleBranch:  *SingleBranch,
				Progress:      os.Stdout,
			}
			if *Recursive {
				PullOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
			}
			err = w.Pull(&PullOptions)
			if err == git.NoErrAlreadyUpToDate {
				color.Green(err.Error())
			} else {
				CheckIfError(err)
			}
		}
	} else if err != nil {
		CheckIfError(err)
	}

	if *LastCommit {
		fmt.Println()

		if r == nil {
			r, err = git.PlainOpen(Destination)
			CheckIfError(err)
		}

		ref, err := r.Head()
		CheckIfError(err)
		color.Green("On branch %s", path.Base(ref.Name().String()))

		commit, err := r.CommitObject(ref.Hash())
		CheckIfError(err)
		fmt.Print("Show last ")
		fmt.Println(commit)
	}
}
