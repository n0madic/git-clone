# git-clone 
[![Build Status](https://travis-ci.org/n0madic/git-clone.svg?branch=master)](https://travis-ci.org/n0madic/git-clone)

Summary
-------
Git clone standalone. Git repository downloader.

Can fetch repositories without installing Git environment!

Help
----
```
usage: git-clone [<flags>] <repository> [<directory>]

Flags:
      --help             Show context-sensitive help (also try --help-long and --help-man).
  -i, --identity=<file>  Selects a file from which the identity (private key) for public key ssh
                         authentication is read. Or use the environment variable GIT_CLONE_KEY for this.
  -r, --recursive        After the clone is created, initialize all submodules within, using their default
                         settings.
  -p, --pull             Incorporates changes from a remote repository into the current branch (if already
                         cloned).
  -o, --origin=<name>    Instead of using the remote name origin to keep track of the upstream repository,
                         use <name>.
  -b, --branch=<name>    Instead of pointing the newly created HEAD to the branch pointed to by the cloned
                         repository’s HEAD, point to <name> branch instead. If the repository already
                         cloned, it will simply switch the branch, and local changes will be discarded.
      --single-branch    Clone only the history leading to the tip of a single branch, either specified by
                         the --branch option or the primary branch remote’s HEAD points at.
  -d, --depth=<depth>    Create a shallow clone with a history truncated to the specified number of
                         commits.
      --tags=all         Tag mode (all|no|following)
  -l, --last             Print the latest commit.

Args:
  <repository>   The repository to clone from.
  [<directory>]  The name of a new directory to clone into.
```

Usage
-----

$ ``git-clone https://github.com/n0madic/git-clone.git``

Clone to another dir:  
$ ``git-clone https://github.com/n0madic/git-clone.git foo``  
$ ``git-clone https://github.com/n0madic/git-clone.git ~/bar``

Cloning specific branch:  
$ ``git-clone -b develop https://github.com/n0madic/git-clone.git``  
Note: if the repository already cloned, it will simply switch the branch, and local changes will be discarded.

Download a specific tag:  
$ ``git-clone -b tags/v0.1.0 https://github.com/n0madic/git-clone.git``  

Pull if repository already cloned (if not then just clone):  
$ ``git-clone --pull https://github.com/n0madic/git-clone.git``

Use custom private key from file:  
$ ``git-clone --identity ~/.ssh/id_rsa.foo git@github.com:n0madic/git-clone.git``

Use custom private key from environment:  
$ ``export GIT_CLONE_KEY=$(cat ~/.ssh/id_rsa.bar)``  
$ ``git-clone git@github.com:n0madic/git-clone.git``  
Note: --identity flag has a higher priority over the environment.
