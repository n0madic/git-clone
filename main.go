package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/fatih/color"
)

var (
	Repository   = kingpin.Arg("repository", "The repository to clone from.").Required().String()
	Directory    = kingpin.Arg("directory", "The name of a new Directory to clone into.").String()
	Recursive    = kingpin.Flag("recursive", "After the clone is created, initialize all submodules within, using their default settings.").Short('r').Bool()
	Pull         = kingpin.Flag("pull", "Incorporates changes from a remote repository into the current branch (if already cloned).").Short('p').Bool()
	RemoteName   = kingpin.Flag("origin", "Instead of using the remote name origin to keep track of the upstream repository, use <name>.").Short('o').PlaceHolder("<name>").Default("origin").String()
	Branch       = kingpin.Flag("branch", "Instead of pointing the newly created HEAD to the branch pointed to by the cloned repository’s HEAD, point to <name> branch instead.").Short('b').PlaceHolder("<name>").String()
	SingleBranch = kingpin.Flag("single-branch", "Clone only the history leading to the tip of a single branch, either specified by the --branch option or the primary branch remote’s HEAD points at.").Bool()
	Depth        = kingpin.Flag("depth", "Create a shallow clone with a history truncated to the specified number of commits.").Short('d').Default("0").Int()
	Tags         = kingpin.Flag("tags", "Tag mode (all|no|following)").Default("all").Enum("all", "no", "following")
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
	if len(*Branch) > 0 {
		CloneOptions.ReferenceName = plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", *Branch))
	}
	if *Recursive {
		CloneOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}
	if *Tags == "all" {
		CloneOptions.Tags = git.AllTags
	} else if *Tags == "no" {
		CloneOptions.Tags = git.NoTags
	} else if *Tags == "following" {
		CloneOptions.Tags = git.TagFollowing
	}

	r, err := git.PlainClone(Destination, false, &CloneOptions)

	if err == git.ErrRepositoryAlreadyExists && *Pull {
		color.Yellow("Repository already exists!")
		r, err := git.PlainOpen(Destination)
		CheckIfError(err)

		w, err := r.Worktree()
		CheckIfError(err)

		color.Cyan("Try pull")
		err = w.Pull(&git.PullOptions{RemoteName: "origin"})
		if err == git.NoErrAlreadyUpToDate {
			color.Green(err.Error())
		} else {
			CheckIfError(err)
		}
	} else if err != nil {
		CheckIfError(err)
	}

	fmt.Println()

	ref, err := r.Head()
	CheckIfError(err)

	commit, err := r.CommitObject(ref.Hash())
	CheckIfError(err)
	fmt.Println("Show last commit:")
	fmt.Println(commit)
}
