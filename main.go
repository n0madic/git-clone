package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/pflag"
)

const (
	exitOK    = 0
	exitFatal = 128
	exitUsage = 129

	noOptSentinel = "__git_clone_noopt__"
)

const cloneUsage = `usage: git clone [<options>] [--] <repo> [<dir>]

    -v, --[no-]verbose    be more verbose
    -q, --[no-]quiet      be more quiet
    --[no-]progress       force progress reporting
    -n, --no-checkout     don't create a checkout
    --checkout            opposite of --no-checkout
    --[no-]bare           create a bare repository
    --[no-]mirror         create a mirror repository (implies --bare)
    -l, --[no-]local      accepted for compatibility; no-op in this build
    --no-hardlinks        accepted for compatibility; no-op in this build
    --hardlinks           accepted for compatibility; no-op in this build
    -s, --[no-]shared     setup as shared repository
    --[no-]recurse-submodules[=<pathspec>]
                          initialize submodules in the clone
    --[no-]recursive[=<pathspec>]
                          alias of --recurse-submodules
    -o, --origin <name>   use <name> instead of 'origin' to track upstream
    -b, --branch <branch> checkout <branch> instead of the remote's HEAD
    --depth <depth>       create a shallow clone of that depth
    --[no-]single-branch  clone only one branch, HEAD or --branch
    --[no-]tags           clone tags, and make later fetches not to follow them
    --[no-]shallow-submodules
                          any cloned submodules will be shallow
    -c, --config <key=value>
                          set config inside the new repository after clone

Unsupported vanilla git clone options that are not implemented by this build are rejected with an error.

Extensions:
    --pull                if destination already exists as a repository, pull instead of failing
    --last                print the latest checked out commit after clone/pull
    --identity <file>     use the given SSH private key file or PEM contents
`

type progressMode int

const (
	progressAuto progressMode = iota
	progressForce
	progressOff
)

type cliError struct {
	code      int
	prefix    string
	message   string
	showUsage bool
	help      bool
}

func (e *cliError) Error() string {
	return e.message
}

type takesNoValueError struct {
	name string
}

func (e *takesNoValueError) Error() string {
	return fmt.Sprintf("option `%s' takes no value", e.name)
}

type presenceValue struct {
	name    string
	present *bool
}

func (v *presenceValue) String() string {
	if v.present == nil || !*v.present {
		return "false"
	}

	return "true"
}

func (v *presenceValue) Set(value string) error {
	if value != noOptSentinel {
		return &takesNoValueError{name: v.name}
	}

	*v.present = true
	return nil
}

func (v *presenceValue) Type() string {
	return "bool"
}

type flagOccurrence struct {
	name  string
	value string
	seq   int
}

type rawOptions struct {
	help                bool
	verbose             bool
	noVerbose           bool
	quiet               bool
	noQuiet             bool
	progress            bool
	noProgress          bool
	rejectShallow       bool
	noRejectShallow     bool
	checkout            bool
	noCheckout          bool
	bare                bool
	noBare              bool
	mirror              bool
	noMirror            bool
	local               bool
	noLocal             bool
	noHardlinks         bool
	hardlinks           bool
	shared              bool
	noShared            bool
	recursive           string
	recurseSubmodules   string
	noRecursive         bool
	noRecurseSubmodules bool
	noJobs              bool
	jobs                string
	template            string
	reference           string
	referenceIfAble     string
	dissociate          bool
	origin              string
	branch              string
	revision            string
	uploadPack          string
	depth               int
	shallowSince        string
	shallowExclude      []string
	singleBranch        bool
	noSingleBranch      bool
	tags                bool
	noTags              bool
	shallowSubmodules   bool
	noShallowSubmodules bool
	separateGitDir      string
	refFormat           string
	configs             []string
	serverOptions       []string
	ipv4                bool
	ipv6                bool
	filter              string
	alsoFilterSubmodule bool
	remoteSubmodules    bool
	sparse              bool
	noSparse            bool
	bundleURI           string
	pull                bool
	last                bool
	identity            string
	occurrences         []flagOccurrence
}

type configEntry struct {
	Section    string
	Subsection string
	Key        string
	Value      string
}

type cloneOptions struct {
	Repository        string
	Directory         string
	RemoteName        string
	Branch            string
	Identity          string
	Depth             int
	Tags              git.TagMode
	Checkout          bool
	Bare              bool
	Mirror            bool
	Shared            bool
	SingleBranch      bool
	RecurseSubmodules bool
	ShallowSubmodules bool
	Quiet             bool
	Verbose           bool
	Progress          progressMode
	Pull              bool
	Last              bool
	ConfigEntries     []configEntry
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseCloneArgs(args)
	if err != nil {
		return renderError(err, stdout, stderr)
	}

	repo, err := executeClone(opts, stderr)
	if err != nil {
		return renderError(err, stdout, stderr)
	}

	if opts.Last {
		if err := printLastCommit(repo, stdout); err != nil {
			return renderError(err, stdout, stderr)
		}
	}

	return exitOK
}

func renderError(err error, stdout, stderr io.Writer) int {
	var cliErr *cliError
	if !errors.As(err, &cliErr) {
		cliErr = &cliError{
			code:    exitFatal,
			prefix:  "fatal",
			message: err.Error(),
		}
	}

	if cliErr.help {
		writeUsage(stdout)
		return cliErr.code
	}

	if cliErr.message != "" {
		fmt.Fprintf(stderr, "%s: %s\n", cliErr.prefix, cliErr.message)
		if cliErr.showUsage && cliErr.prefix == "fatal" {
			fmt.Fprintln(stderr)
		}
	}

	if cliErr.showUsage {
		writeUsage(stderr)
	}

	return cliErr.code
}

func writeUsage(w io.Writer) {
	_, _ = io.WriteString(w, cloneUsage)
}

func parseCloneArgs(args []string) (cloneOptions, error) {
	raw := rawOptions{
		origin:   git.DefaultRemoteName,
		identity: os.Getenv("GIT_CLONE_KEY"),
	}

	flagSet := newCloneFlagSet(&raw)
	if err := parseWithOccurrences(flagSet, &raw, args); err != nil {
		return cloneOptions{}, classifyParseError(err)
	}

	if raw.help {
		return cloneOptions{}, &cliError{code: exitUsage, help: true}
	}

	return finalizeOptions(&raw, flagSet.Args())
}

func newCloneFlagSet(raw *rawOptions) *pflag.FlagSet {
	fs := pflag.NewFlagSet("git-clone", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SetInterspersed(false)
	fs.SortFlags = false
	fs.Usage = func() {}

	addPresenceFlag(fs, &raw.help, "help", "h", "")
	addPresenceFlag(fs, &raw.verbose, "verbose", "v", "")
	addPresenceFlag(fs, &raw.noVerbose, "no-verbose", "", "")
	addPresenceFlag(fs, &raw.quiet, "quiet", "q", "")
	addPresenceFlag(fs, &raw.noQuiet, "no-quiet", "", "")
	addPresenceFlag(fs, &raw.progress, "progress", "", "")
	addPresenceFlag(fs, &raw.noProgress, "no-progress", "", "")
	addPresenceFlag(fs, &raw.rejectShallow, "reject-shallow", "", "")
	addPresenceFlag(fs, &raw.noRejectShallow, "no-reject-shallow", "", "")
	addPresenceFlag(fs, &raw.checkout, "checkout", "", "")
	addPresenceFlag(fs, &raw.noCheckout, "no-checkout", "n", "")
	addPresenceFlag(fs, &raw.bare, "bare", "", "")
	addPresenceFlag(fs, &raw.noBare, "no-bare", "", "")
	addPresenceFlag(fs, &raw.mirror, "mirror", "", "")
	addPresenceFlag(fs, &raw.noMirror, "no-mirror", "", "")
	addPresenceFlag(fs, &raw.local, "local", "l", "")
	addPresenceFlag(fs, &raw.noLocal, "no-local", "", "")
	addPresenceFlag(fs, &raw.hardlinks, "hardlinks", "", "")
	addPresenceFlag(fs, &raw.noHardlinks, "no-hardlinks", "", "")
	addPresenceFlag(fs, &raw.shared, "shared", "s", "")
	addPresenceFlag(fs, &raw.noShared, "no-shared", "", "")
	addOptionalStringFlag(fs, &raw.recursive, "recursive", "", "")
	addOptionalStringFlag(fs, &raw.recurseSubmodules, "recurse-submodules", "", "")
	addPresenceFlag(fs, &raw.noRecursive, "no-recursive", "", "")
	addPresenceFlag(fs, &raw.noRecurseSubmodules, "no-recurse-submodules", "", "")
	addPresenceFlag(fs, &raw.noJobs, "no-jobs", "", "")
	fs.StringVarP(&raw.jobs, "jobs", "j", "", "")
	fs.StringVar(&raw.template, "template", "", "")
	fs.StringVar(&raw.reference, "reference", "", "")
	fs.StringVar(&raw.referenceIfAble, "reference-if-able", "", "")
	addPresenceFlag(fs, &raw.dissociate, "dissociate", "", "")
	fs.StringVarP(&raw.origin, "origin", "o", git.DefaultRemoteName, "")
	fs.StringVarP(&raw.branch, "branch", "b", "", "")
	fs.StringVar(&raw.revision, "revision", "", "")
	fs.StringVarP(&raw.uploadPack, "upload-pack", "u", "", "")
	fs.IntVar(&raw.depth, "depth", 0, "")
	fs.StringVar(&raw.shallowSince, "shallow-since", "", "")
	fs.StringArrayVar(&raw.shallowExclude, "shallow-exclude", nil, "")
	addPresenceFlag(fs, &raw.singleBranch, "single-branch", "", "")
	addPresenceFlag(fs, &raw.noSingleBranch, "no-single-branch", "", "")
	addPresenceFlag(fs, &raw.tags, "tags", "", "")
	addPresenceFlag(fs, &raw.noTags, "no-tags", "", "")
	addPresenceFlag(fs, &raw.shallowSubmodules, "shallow-submodules", "", "")
	addPresenceFlag(fs, &raw.noShallowSubmodules, "no-shallow-submodules", "", "")
	fs.StringVar(&raw.separateGitDir, "separate-git-dir", "", "")
	fs.StringVar(&raw.refFormat, "ref-format", "", "")
	fs.StringArrayVarP(&raw.configs, "config", "c", nil, "")
	fs.StringArrayVar(&raw.serverOptions, "server-option", nil, "")
	addPresenceFlag(fs, &raw.ipv4, "ipv4", "4", "")
	addPresenceFlag(fs, &raw.ipv6, "ipv6", "6", "")
	fs.StringVar(&raw.filter, "filter", "", "")
	addPresenceFlag(fs, &raw.alsoFilterSubmodule, "also-filter-submodules", "", "")
	addPresenceFlag(fs, &raw.remoteSubmodules, "remote-submodules", "", "")
	addPresenceFlag(fs, &raw.sparse, "sparse", "", "")
	addPresenceFlag(fs, &raw.noSparse, "no-sparse", "", "")
	fs.StringVar(&raw.bundleURI, "bundle-uri", "", "")

	addPresenceFlag(fs, &raw.pull, "pull", "", "")
	addPresenceFlag(fs, &raw.last, "last", "", "")
	fs.StringVar(&raw.identity, "identity", raw.identity, "")

	return fs
}

func addPresenceFlag(fs *pflag.FlagSet, target *bool, name, shorthand, usage string) {
	flag := fs.VarPF(&presenceValue{name: name, present: target}, name, shorthand, usage)
	flag.NoOptDefVal = noOptSentinel
}

func addOptionalStringFlag(fs *pflag.FlagSet, target *string, name, shorthand, usage string) {
	if shorthand == "" {
		fs.StringVar(target, name, "", usage)
	} else {
		fs.StringVarP(target, name, shorthand, "", usage)
	}

	fs.Lookup(name).NoOptDefVal = noOptSentinel
}

func parseWithOccurrences(fs *pflag.FlagSet, raw *rawOptions, args []string) error {
	seq := 0

	return fs.ParseAll(args, func(flag *pflag.Flag, value string) error {
		raw.occurrences = append(raw.occurrences, flagOccurrence{
			name:  flag.Name,
			value: value,
			seq:   seq,
		})
		seq++

		return fs.Set(flag.Name, value)
	})
}

func classifyParseError(err error) error {
	if errors.Is(err, pflag.ErrHelp) {
		return &cliError{code: exitUsage, help: true}
	}

	var noValue *takesNoValueError
	if errors.As(err, &noValue) {
		return &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   noValue.Error(),
			showUsage: true,
		}
	}

	var notExist *pflag.NotExistError
	if errors.As(err, &notExist) {
		if notExist.GetSpecifiedShortnames() != "" {
			return &cliError{
				code:      exitUsage,
				prefix:    "error",
				message:   fmt.Sprintf("unknown switch `%s'", notExist.GetSpecifiedName()),
				showUsage: true,
			}
		}

		return &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   fmt.Sprintf("unknown option `%s'", notExist.GetSpecifiedName()),
			showUsage: true,
		}
	}

	var valueRequired *pflag.ValueRequiredError
	if errors.As(err, &valueRequired) {
		name := valueRequired.GetSpecifiedName()
		if valueRequired.GetSpecifiedShortnames() != "" {
			return &cliError{
				code:      exitUsage,
				prefix:    "error",
				message:   fmt.Sprintf("switch `%s' requires a value", name),
				showUsage: true,
			}
		}

		return &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   fmt.Sprintf("option `%s' requires a value", name),
			showUsage: true,
		}
	}

	var invalidValue *pflag.InvalidValueError
	if errors.As(err, &invalidValue) {
		return &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   invalidValue.Error(),
			showUsage: true,
		}
	}

	return &cliError{
		code:      exitUsage,
		prefix:    "error",
		message:   err.Error(),
		showUsage: true,
	}
}

func finalizeOptions(raw *rawOptions, positionals []string) (cloneOptions, error) {
	if len(positionals) == 0 {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "fatal",
			message:   "You must specify a repository to clone.",
			showUsage: true,
		}
	}

	if len(positionals) > 2 {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "fatal",
			message:   "Too many arguments.",
			showUsage: true,
		}
	}

	if unsupported := firstUnsupportedFlag(raw.occurrences); unsupported != nil {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   fmt.Sprintf("option `%s' is not supported by this build of git-clone", unsupported.name),
			showUsage: true,
		}
	}

	if seen(raw.occurrences, "depth") && raw.depth <= 0 {
		return cloneOptions{}, &cliError{
			code:    exitFatal,
			prefix:  "fatal",
			message: fmt.Sprintf("depth %d is not a positive number", raw.depth),
		}
	}

	if seen(raw.occurrences, "origin") && raw.origin == "" {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   "option `origin' requires a non-empty value",
			showUsage: true,
		}
	}

	if seen(raw.occurrences, "branch") && raw.branch == "" {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   "option `branch' requires a non-empty value",
			showUsage: true,
		}
	}

	if seen(raw.occurrences, "identity") && raw.identity == "" {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   "option `identity' requires a non-empty value",
			showUsage: true,
		}
	}

	recurseSubmodules, recurseOccurrence := resolveOptionalToggle(
		raw.occurrences,
		false,
		[]string{"recursive", "recurse-submodules"},
		[]string{"no-recursive", "no-recurse-submodules"},
	)
	if recurseSubmodules && recurseOccurrence != nil && recurseOccurrence.value != noOptSentinel {
		return cloneOptions{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   fmt.Sprintf("option `%s' pathspec is not supported by this build of git-clone", recurseOccurrence.name),
			showUsage: true,
		}
	}

	configEntries, err := parseConfigEntries(raw.configs)
	if err != nil {
		return cloneOptions{}, err
	}

	quiet := resolveToggle(raw.occurrences, false, []string{"quiet"}, []string{"no-quiet"})
	progress := resolveProgressMode(raw.occurrences, quiet)
	checkout := resolveToggle(raw.occurrences, true, []string{"checkout"}, []string{"no-checkout"})
	mirror := resolveToggle(raw.occurrences, false, []string{"mirror"}, []string{"no-mirror"})
	bare := resolveToggle(raw.occurrences, false, []string{"bare"}, []string{"no-bare"}) || mirror
	shared := resolveToggle(raw.occurrences, false, []string{"shared"}, []string{"no-shared"})
	singleBranch := resolveToggle(raw.occurrences, false, []string{"single-branch"}, []string{"no-single-branch"})
	tags := resolveToggle(raw.occurrences, true, []string{"tags"}, []string{"no-tags"})
	shallowSubmodules := resolveToggle(raw.occurrences, false, []string{"shallow-submodules"}, []string{"no-shallow-submodules"})
	verbose := resolveToggle(raw.occurrences, false, []string{"verbose"}, []string{"no-verbose"})
	_ = resolveToggle(raw.occurrences, true, []string{"local"}, []string{"no-local"})
	_ = resolveToggle(raw.occurrences, true, []string{"hardlinks"}, []string{"no-hardlinks"})

	return cloneOptions{
		Repository:        positionals[0],
		Directory:         positionalDirectory(positionals),
		RemoteName:        raw.origin,
		Branch:            raw.branch,
		Identity:          raw.identity,
		Depth:             raw.depth,
		Tags:              boolToTagMode(tags),
		Checkout:          checkout,
		Bare:              bare,
		Mirror:            mirror,
		Shared:            shared,
		SingleBranch:      singleBranch,
		RecurseSubmodules: recurseSubmodules,
		ShallowSubmodules: shallowSubmodules,
		Quiet:             quiet,
		Verbose:           verbose,
		Progress:          progress,
		Pull:              raw.pull,
		Last:              raw.last,
		ConfigEntries:     configEntries,
	}, nil
}

func positionalDirectory(positionals []string) string {
	if len(positionals) > 1 {
		return positionals[1]
	}

	return ""
}

func parseConfigEntries(rawEntries []string) ([]configEntry, error) {
	entries := make([]configEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		entry, err := parseConfigEntry(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func parseConfigEntry(raw string) (configEntry, error) {
	idx := strings.IndexByte(raw, '=')
	if idx <= 0 {
		return configEntry{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   "option `config' expects `section.key=value' or `section.subsection.key=value'",
			showUsage: true,
		}
	}

	keyPath := raw[:idx]
	value := raw[idx+1:]
	firstDot := strings.IndexByte(keyPath, '.')
	lastDot := strings.LastIndexByte(keyPath, '.')
	if firstDot <= 0 || lastDot <= firstDot || lastDot == len(keyPath)-1 {
		if firstDot > 0 && firstDot == lastDot && firstDot < len(keyPath)-1 {
			return configEntry{
				Section: keyPath[:firstDot],
				Key:     keyPath[firstDot+1:],
				Value:   value,
			}, nil
		}

		return configEntry{}, &cliError{
			code:      exitUsage,
			prefix:    "error",
			message:   fmt.Sprintf("invalid config key `%s`", keyPath),
			showUsage: true,
		}
	}

	return configEntry{
		Section:    keyPath[:firstDot],
		Subsection: keyPath[firstDot+1 : lastDot],
		Key:        keyPath[lastDot+1:],
		Value:      value,
	}, nil
}

func boolToTagMode(enabled bool) git.TagMode {
	if enabled {
		return git.AllTags
	}

	return git.NoTags
}

func resolveProgressMode(occurrences []flagOccurrence, quiet bool) progressMode {
	if quiet {
		return progressOff
	}

	progress := lastOccurrence(occurrences, "progress")
	noProgress := lastOccurrence(occurrences, "no-progress")
	if progress == nil && noProgress == nil {
		return progressAuto
	}

	if noProgress != nil && (progress == nil || noProgress.seq > progress.seq) {
		return progressOff
	}

	return progressForce
}

func resolveToggle(occurrences []flagOccurrence, defaultValue bool, positive, negative []string) bool {
	pos := lastOccurrence(occurrences, positive...)
	neg := lastOccurrence(occurrences, negative...)
	if pos == nil && neg == nil {
		return defaultValue
	}

	if neg != nil && (pos == nil || neg.seq > pos.seq) {
		return false
	}

	return true
}

func resolveOptionalToggle(
	occurrences []flagOccurrence,
	defaultValue bool,
	positive, negative []string,
) (bool, *flagOccurrence) {
	pos := lastOccurrence(occurrences, positive...)
	neg := lastOccurrence(occurrences, negative...)
	if pos == nil && neg == nil {
		return defaultValue, nil
	}

	if neg != nil && (pos == nil || neg.seq > pos.seq) {
		return false, nil
	}

	return true, pos
}

func seen(occurrences []flagOccurrence, name string) bool {
	return lastOccurrence(occurrences, name) != nil
}

func lastOccurrence(occurrences []flagOccurrence, names ...string) *flagOccurrence {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	var result *flagOccurrence
	for i := range occurrences {
		occurrence := &occurrences[i]
		if _, ok := nameSet[occurrence.name]; !ok {
			continue
		}
		if result == nil || occurrence.seq > result.seq {
			result = occurrence
		}
	}

	return result
}

func firstUnsupportedFlag(occurrences []flagOccurrence) *flagOccurrence {
	unsupported := map[string]struct{}{
		"reject-shallow":         {},
		"no-reject-shallow":      {},
		"jobs":                   {},
		"no-jobs":                {},
		"template":               {},
		"reference":              {},
		"reference-if-able":      {},
		"dissociate":             {},
		"revision":               {},
		"upload-pack":            {},
		"shallow-since":          {},
		"shallow-exclude":        {},
		"separate-git-dir":       {},
		"ref-format":             {},
		"server-option":          {},
		"ipv4":                   {},
		"ipv6":                   {},
		"filter":                 {},
		"also-filter-submodules": {},
		"remote-submodules":      {},
		"sparse":                 {},
		"no-sparse":              {},
		"bundle-uri":             {},
	}

	for i := range occurrences {
		occurrence := &occurrences[i]
		if _, ok := unsupported[occurrence.name]; ok {
			return occurrence
		}
	}

	return nil
}

func destinationFor(opts cloneOptions) string {
	if opts.Directory != "" {
		return opts.Directory
	}

	endpoint, err := gitEndpointPath(opts.Repository)
	if err == nil {
		base := path.Base(endpoint)
		if base != "." && base != "/" && base != "" {
			return strings.TrimSuffix(base, ".git")
		}
	}

	repository := strings.TrimRight(opts.Repository, "/")
	base := filepath.Base(repository)
	return strings.TrimSuffix(base, ".git")
}

func gitEndpointPath(repository string) (string, error) {
	endpoint, err := gitTransportEndpoint(repository)
	if err != nil {
		return "", err
	}

	return endpoint.Path, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
