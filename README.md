# git-clone

`git-clone` is a standalone repository downloader built on top of `go-git`. It is intended to be a practical drop-in replacement for common `git clone` workflows without requiring a full Git installation.

Where `go-git` supports the underlying behavior directly, the utility follows Git-compatible argument names, exit codes, destination checks, and branch or tag checkout semantics. Features that cannot be reproduced faithfully are either accepted as compatibility no-ops or rejected early with a git-like error.

## Compatibility

### Supported

- `repo [dir]`
- `-v/--verbose`
- `-q/--quiet`
- `--progress`, `--no-progress`
- `-n/--no-checkout`, `--checkout`
- `--bare`, `--no-bare`
- `--mirror`, `--no-mirror`
- `-s/--shared`, `--no-shared`
- `--recursive`, `--no-recursive`
- `--recurse-submodules`, `--no-recurse-submodules`
- `-o/--origin`
- `-b/--branch`
- `--single-branch`, `--no-single-branch`
- `--depth`
- `--tags`, `--no-tags`
- `--shallow-submodules`, `--no-shallow-submodules`

### Supported As Compatibility No-Ops

These flags are accepted to preserve CLI compatibility, but this build does not emulate Git's transport/storage optimization behind them:

- `-l/--local`, `--no-local`
- `--hardlinks`, `--no-hardlinks`

### Partial Support

- `-c/--config key=value`
  - applied to the local repository config after clone/pull
  - does not influence transport-time behavior the way vanilla Git can
- `--recursive[=<pathspec>]` / `--recurse-submodules[=<pathspec>]`
  - supported without a pathspec
  - pathspec form is rejected early as unsupported
- `-b/--branch`
  - supports both branches and tags
  - if a branch and tag share the same name, branch wins like vanilla Git

### Recognized But Unsupported

These options are parsed and fail early with exit code `129` and a git-like error:

- `--reject-shallow`, `--no-reject-shallow`
- `-j/--jobs`, `--no-jobs`
- `--template`
- `--reference`
- `--reference-if-able`
- `--dissociate`
- `--revision`
- `-u/--upload-pack`
- `--shallow-since`
- `--shallow-exclude`
- `--separate-git-dir`
- `--ref-format`
- `--server-option`
- `-4/--ipv4`
- `-6/--ipv6`
- `--filter`
- `--also-filter-submodules`
- `--remote-submodules`
- `--sparse`, `--no-sparse`
- `--bundle-uri`

## Extensions

These are intentionally outside vanilla `git clone`, but remain available as long-only flags so they do not collide with Git's standard short options:

- `--pull`
  - if destination already exists as a repository, pull instead of failing
  - plain clone behavior remains Git-compatible unless `--pull` is explicitly set
- `--last`
  - prints the latest checked out commit after clone/pull
- `--identity <file>`
  - uses the given SSH private key file or PEM contents
  - also respects `GIT_CLONE_KEY`

## Behavioral Notes

- Exit codes follow Git-style conventions:
  - `0` success
  - `128` fatal runtime/semantic failure
  - `129` usage, help, unknown option, unsupported option
- Progress is written to `stderr`, not `stdout`.
- Progress is shown automatically only when `stderr` is a terminal, unless `--progress` forces it or `--quiet` / `--no-progress` disables it.
- Existing non-empty destinations now fail like vanilla `git clone`.
- Existing repositories are only mutated when `--pull` is explicitly used.

## Examples

```bash
git-clone https://github.com/n0madic/git-clone.git
git-clone -b main https://github.com/n0madic/git-clone.git dst
git-clone --no-tags --depth 1 https://github.com/n0madic/git-clone.git
git-clone --recursive https://github.com/n0madic/git-clone.git
git-clone --pull https://github.com/n0madic/git-clone.git
git-clone --identity ~/.ssh/id_ed25519 git@github.com:n0madic/git-clone.git
```
