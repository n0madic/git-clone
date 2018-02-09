package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/src-d/go-git.v4"
)

var (
	Directory   string
	Repository  = kingpin.Arg("repository", "The repository to clone from.").Required().String()
	Destination = kingpin.Arg("directory", "The name of a new Directory to clone into.").String()
	Recursive   = kingpin.Flag("recursive", "After the clone is created, initialize all submodules within, using their default settings.").Short('r').Bool()
)

func CheckIfError(err error) {
	if err == nil {
		return
	}

	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}

func main() {

	kingpin.Parse()

	if len(*Destination) > 0 {
		Directory = *Destination
	} else {
		Directory = strings.TrimRight(path.Base(*Repository), ".git")
	}

	RecurseSubmodulesFlag := git.NoRecurseSubmodules
	if *Recursive {
		RecurseSubmodulesFlag = git.DefaultSubmoduleRecursionDepth
	}

	r, err := git.PlainClone(Directory, false, &git.CloneOptions{
		URL:               *Repository,
		RecurseSubmodules: RecurseSubmodulesFlag,
		Progress:          os.Stdout,
	})
	CheckIfError(err)

	fmt.Println()

	ref, err := r.Head()
	CheckIfError(err)

	commit, err := r.CommitObject(ref.Hash())
	CheckIfError(err)
	fmt.Println("Show last commit:")
	fmt.Println(commit)
}
