# git-clone
Git clone standalone
```
usage: git-clone [<flags>] <repository> [<directory>]

Flags:
      --help           Show context-sensitive help (also try --help-long and --help-man).
  -r, --recursive      After the clone is created, initialize all submodules within, using their default settings.
  -p, --pull           Incorporates changes from a remote repository into the current branch (if already cloned).
  -o, --origin=<name>  Instead of using the remote name origin to keep track of the upstream repository, use <name>.
  -b, --branch=<name>  Instead of pointing the newly created HEAD to the branch pointed to by the cloned repository’s HEAD, point to <name> branch instead. If the repository already
                       cloned, it will simply switch the branch, and local changes will be discarded.
      --single-branch  Clone only the history leading to the tip of a single branch, either specified by the --branch option or the primary branch remote’s HEAD points at.
  -d, --depth=<depth>  Create a shallow clone with a history truncated to the specified number of commits.
      --tags=all       Tag mode (all|no|following)
  -l, --last           Print the latest commit.

Args:
  <repository>   The repository to clone from.
  [<directory>]  The name of a new Directory to clone into.
```
