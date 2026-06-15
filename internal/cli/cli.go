package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/orient"
	"github.com/tpisel/memento/internal/setup"
	"github.com/tpisel/memento/internal/vault"
)

var version = "dev"

var writeCompileArtifactsAfterWrite = writeCompileArtifacts

const helpText = `memento

Usage:
  memento help
  memento version
  memento brief
  memento compile
  memento init [--dir <vault>]
  memento orient
  memento read <key|@N>
  memento write [--overwrite] <key>

Commands:
  help      Show this help text.
  version   Print the memento version.
  brief     Print the agent-facing manifest projection.
  compile   Compile a memory vault manifest.
  init      Adopt or create a memory vault.
  orient    Print tool-usage orientation and project overlays.
  read      Read a memory note by key or @N entry reference.
  write     Create, append to, or overwrite a memory note from stdin, then compile.
`

// Run dispatches the CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, os.Stdin, stdout, stderr)
}

// RunWithInput dispatches the CLI using stdin for commands that consume a body.
func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, helpText)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "memento %s\n", version)
		return 0
	case "brief":
		return runBrief(args[1:], stdout, stderr)
	case "compile":
		return runCompile(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "orient":
		return runOrient(args[1:], stdout, stderr)
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "write":
		return runWrite(args[1:], stdin, stdout, stderr)
	default:
		printRootError(stderr, fmt.Errorf("%w %q", ErrUnknownCommand, args[0]))
		return 2
	}
}

func runOrient(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("orient", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "orient", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "orient", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}

	m, err := readManifest(v)
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}
	data, err := orient.Render(v, m)
	if err != nil {
		printCLIError(stderr, "orient", err)
		return 1
	}
	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "orient", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}

func runBrief(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("brief", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "brief", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "brief", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "brief", err)
		return 1
	}

	data, err := readOrRenderBrief(v)
	if err != nil {
		printCLIError(stderr, "brief", err)
		return 1
	}
	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "brief", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}

func readOrRenderBrief(v vault.Vault) ([]byte, error) {
	path := vault.BriefPath(v)
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read brief: %w", err)
	}

	m, err := manifest.Load(v)
	if err != nil {
		return nil, err
	}
	if err := writeBriefArtifact(v, m); err != nil {
		return nil, err
	}

	data, err = os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rendered brief: %w", err)
	}
	return data, nil
}

func runCompile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "compile", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "compile", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "compile", err)
		return 1
	}

	warnings, _, err := writeCompileArtifacts(v)
	if err != nil {
		printCLIError(stderr, "compile", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)
	return 0
}

func writeCompileArtifacts(v vault.Vault) ([]manifest.Warning, int, error) {
	m, warnings, err := manifest.CompileWithWarnings(v)
	if err != nil {
		return nil, 0, err
	}

	data, err := manifest.Marshal(m)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(v.MarkerDir, 0o755); err != nil {
		return nil, 0, fmt.Errorf("create marker directory: %w", err)
	}
	if err := os.WriteFile(v.ManifestPath, data, 0o644); err != nil {
		return nil, 0, fmt.Errorf("write manifest: %w", err)
	}

	if err := writeBriefArtifact(v, m); err != nil {
		warnings = append(warnings, manifest.Warning{Path: filepath.ToSlash(filepath.Join(vault.ToolDirName, vault.BriefFileName)), Err: err})
	}
	return warnings, len(m.Entries), nil
}

func writeBriefArtifact(v vault.Vault, m manifest.Manifest) error {
	toolFiles, err := brief.DetectToolFiles(v)
	if err != nil {
		return err
	}
	path := vault.BriefPath(v)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create brief directory: %w", err)
	}
	if err := os.WriteFile(path, brief.RenderWithToolFiles(m, toolFiles), 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}
	return nil
}

func printCompileWarnings(stderr io.Writer, warnings []manifest.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "memento compile: warning: %v\n", warning)
	}
}

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dir := flags.String("dir", "", "memory vault directory")
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "init", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 0 {
		printCLIError(stderr, "init", fmt.Errorf("%w: unexpected argument %q", ErrInvalidArguments, flags.Arg(0)))
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		printCLIError(stderr, "init", fmt.Errorf("%w: get current directory: %v", ErrIO, err))
		return 1
	}

	v, err := setup.Init(wd, *dir)
	if err != nil {
		printCLIError(stderr, "init", err)
		return 1
	}
	fmt.Fprintf(stdout, "Initialized memento vault: %s\n", v.Root)
	return 0
}

func runRead(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("read", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "read", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 1 {
		printCLIError(stderr, "read", fmt.Errorf("%w: expected exactly one key or @N entry reference", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}

	target := flags.Arg(0)
	var data []byte
	var key string
	var numberedManifest manifest.Manifest
	if strings.HasPrefix(target, "@") {
		data, key, numberedManifest, err = readNumberedEntry(v, strings.TrimPrefix(target, "@"))
	} else {
		data, err = note.Read(v, target)
		key = target
	}
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}
	readKey := key
	if fileKey, _, hasSection := strings.Cut(readKey, "#"); hasSection {
		readKey = fileKey
	}

	binding, err := note.BindingForReadTarget(v, key)
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}
	fmt.Fprintf(stderr, "binding: %s\n", binding)

	linkManifest, ok, err := manifestForReadLinks(v, target, numberedManifest)
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}
	if ok {
		var lines []string
		if _, section, hasSection := strings.Cut(target, "#"); hasSection {
			lines = readSectionLinkSurfaceLines(linkManifest, readKey, section, data, sectionInlinksAnchorFiltered)
		} else {
			lines = readLinkSurfaceLines(linkManifest, readKey)
		}
		for _, line := range lines {
			fmt.Fprintln(stderr, line)
		}
	}
	if strings.HasPrefix(target, "@") {
		warnIfBriefHashDrift(v, numberedManifest, stderr)
	}

	if _, err := stdout.Write(data); err != nil {
		printCLIError(stderr, "read", fmt.Errorf("%w: write stdout: %v", ErrIO, err))
		return 1
	}
	return 0
}

func manifestForReadLinks(v vault.Vault, target string, numberedManifest manifest.Manifest) (manifest.Manifest, bool, error) {
	if strings.HasPrefix(target, "@") {
		return numberedManifest, true, nil
	}

	m, err := readManifest(v)
	if err != nil {
		if errors.Is(err, manifest.ErrNotFound) {
			return manifest.Manifest{}, false, nil
		}
		return manifest.Manifest{}, false, err
	}
	return m, true, nil
}

func readLinkSurfaceLines(m manifest.Manifest, key string) []string {
	entry, ok := manifestEntryByKey(m, key)
	if !ok {
		return nil
	}
	return readLinkSurfaceLinesForEntry(m, entry)
}

func readLinkSurfaceLinesForEntry(m manifest.Manifest, entry manifest.Entry) []string {
	numberByKey := map[string]int{}
	for _, numbered := range brief.NumberedEntries(m) {
		numberByKey[numbered.Entry.Key] = numbered.Number
	}

	lines := []string{}
	if entries := linkSurfaceInEntries(entry.Links.In, numberByKey, "wiki"); len(entries) > 0 {
		lines = append(lines, "inlinks: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceOutEntries(entry.Links.Out, numberByKey, "wiki"); len(entries) > 0 {
		lines = append(lines, "outlinks: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceOutEntries(entry.Links.Out, numberByKey, "embed"); len(entries) > 0 {
		lines = append(lines, "transcludes: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceInEntries(entry.Links.In, numberByKey, "embed"); len(entries) > 0 {
		lines = append(lines, "transcluded-by: "+strings.Join(entries, ", "))
	}
	return lines
}

type sectionInlinksMode int

const (
	sectionInlinksAnchorFiltered sectionInlinksMode = iota
	sectionInlinksFileScoped
)

func readSectionLinkSurfaceLines(m manifest.Manifest, key, section string, data []byte, mode sectionInlinksMode) []string {
	entry, ok := manifestEntryByKey(m, key)
	if !ok {
		return nil
	}

	sectionEntry := entry
	sectionEntry.Links.Out = sectionOutLinks(m, key, data)
	if mode == sectionInlinksAnchorFiltered {
		sectionSlug, ok := entrySectionSlug(entry, section)
		if !ok {
			sectionSlug = section
		}
		sectionEntry.Links.In = sectionInLinks(entry, sectionSlug)
	}

	return readLinkSurfaceLinesForEntry(m, sectionEntry)
}

func sectionOutLinks(m manifest.Manifest, currentKey string, source []byte) []manifest.OutLink {
	resolver := manifest.NewWikiLinkResolver(m.Entries)
	rawLinks := markdown.ExtractWikiLinks(source)
	links := make([]sectionOutLink, 0, len(rawLinks))
	seen := map[string]bool{}

	for _, raw := range rawLinks {
		target, resolved := resolver.Resolve(currentKey, raw.Target)
		link := manifest.OutLink{
			Target:   target,
			Type:     raw.Type,
			Resolved: resolved,
			Anchor:   raw.Anchor,
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%t", link.Target, link.Type, link.Anchor, link.Resolved)
		if seen[key] {
			continue
		}
		seen[key] = true
		links = append(links, sectionOutLink{OutLink: link, occurrence: raw.Occurrence})
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].Target != links[j].Target {
			return links[i].Target < links[j].Target
		}
		if links[i].occurrence != links[j].occurrence {
			return links[i].occurrence < links[j].occurrence
		}
		if links[i].Anchor != links[j].Anchor {
			return links[i].Anchor < links[j].Anchor
		}
		return links[i].Type < links[j].Type
	})

	out := make([]manifest.OutLink, 0, len(links))
	for _, link := range links {
		out = append(out, link.OutLink)
	}
	return out
}

type sectionOutLink struct {
	manifest.OutLink
	occurrence int
}

func sectionInLinks(entry manifest.Entry, sectionSlug string) []manifest.InLink {
	links := []manifest.InLink{}
	for _, link := range entry.Links.In {
		if link.Anchor == "" {
			continue
		}
		if inLinkAnchorMatchesSection(entry, link.Anchor, sectionSlug) {
			links = append(links, link)
		}
	}
	return links
}

func entrySectionSlug(entry manifest.Entry, section string) (string, bool) {
	for _, heading := range entry.Headings {
		if section == heading.Slug || section == heading.Text {
			return heading.Slug, true
		}
	}
	return "", false
}

func inLinkAnchorMatchesSection(entry manifest.Entry, anchor, sectionSlug string) bool {
	for _, heading := range entry.Headings {
		if heading.Slug != sectionSlug {
			continue
		}
		return anchor == heading.Slug || anchor == heading.Text
	}
	return anchor == sectionSlug
}

func manifestEntryByKey(m manifest.Manifest, key string) (manifest.Entry, bool) {
	for _, entry := range m.Entries {
		if entry.Key == key {
			return entry, true
		}
	}
	return manifest.Entry{}, false
}

func linkSurfaceOutEntries(links []manifest.OutLink, numberByKey map[string]int, linkType string) []string {
	entries := []string{}
	for _, link := range links {
		if string(link.Type) != linkType {
			continue
		}
		target := link.Target
		if !link.Resolved && link.Anchor != "" {
			target += "#" + link.Anchor
		}
		entries = append(entries, linkSurfaceEntry(target, link.Resolved, numberByKey))
	}
	return entries
}

func linkSurfaceInEntries(links []manifest.InLink, numberByKey map[string]int, linkType string) []string {
	entries := []string{}
	for _, link := range links {
		if string(link.Type) != linkType {
			continue
		}
		entries = append(entries, linkSurfaceEntry(link.Source, true, numberByKey))
	}
	return entries
}

func linkSurfaceEntry(key string, resolved bool, numberByKey map[string]int) string {
	number, ok := numberByKey[key]
	if resolved && ok {
		return fmt.Sprintf("%s @%d", key, number)
	}
	return key
}

func readNumberedEntry(v vault.Vault, target string) ([]byte, string, manifest.Manifest, error) {
	number, err := strconv.Atoi(target)
	if err != nil {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry reference must be @ followed by a number: @%s", ErrInvalidEntryReference, target)
	}
	if number < 1 {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry number must be 1 or greater: @%s", ErrNumericOutOfRange, target)
	}

	m, err := readManifest(v)
	if err != nil {
		return nil, "", manifest.Manifest{}, err
	}

	numbered := brief.NumberedEntries(m)
	if number > len(numbered) {
		return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry %d does not exist in manifest; manifest has %d entries", ErrNumericOutOfRange, number, len(numbered))
	}

	key := numbered[number-1].Entry.Key
	data, err := note.Read(v, key)
	if err != nil {
		if errors.Is(err, note.ErrNotFound) {
			return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry %d's file `%s` no longer exists", manifest.ErrStale, number, key)
		}
		return nil, "", manifest.Manifest{}, err
	}

	return data, key, m, nil
}

func readManifest(v vault.Vault) (manifest.Manifest, error) {
	return manifest.Load(v)
}

func warnIfBriefHashDrift(v vault.Vault, m manifest.Manifest, stderr io.Writer) {
	data, err := os.ReadFile(vault.BriefPath(v))
	if err != nil {
		return
	}

	briefHash, ok := briefManifestHash(data)
	if !ok || briefHash == brief.ManifestHash(m) {
		return
	}
	fmt.Fprintln(stderr, "warn: manifest changed since last brief, numbers may not match your view — re-run memento brief.")
}

func briefManifestHash(data []byte) (string, bool) {
	text := string(data)
	line, rest, ok := strings.Cut(text, "\n")
	if !ok || strings.TrimSuffix(line, "\r") != "---" {
		return "", false
	}

	for {
		line, next, hasNext := strings.Cut(rest, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "---" {
			return "", false
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "manifest" {
			return strings.TrimSpace(value), true
		}
		if !hasNext {
			return "", false
		}
		rest = next
	}
}

func runWrite(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("write", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	overwrite := flags.Bool("overwrite", false, "replace the target note with stdin instead of appending")
	if err := flags.Parse(args); err != nil {
		printCLIError(stderr, "write", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
		return 2
	}
	if flags.NArg() != 1 {
		printCLIError(stderr, "write", fmt.Errorf("%w: expected exactly one key", ErrInvalidArguments))
		return 2
	}

	v, err := resolveVault()
	if err != nil {
		printCLIError(stderr, "write", err)
		return 1
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		printCLIError(stderr, "write", fmt.Errorf("%w: read stdin: %v", ErrIO, err))
		return 1
	}

	opts := note.WriteOptions{}
	if *overwrite {
		opts.Operation = note.OperationOverwrite
	}
	if err := note.Write(v, flags.Arg(0), data, opts); err != nil {
		printCLIError(stderr, "write", err)
		return 1
	}
	warnings, count, err := writeCompileArtifactsAfterWrite(v)
	if err != nil {
		fmt.Fprintf(stderr, "memento write: warning: recompile failed after successful write: %v\n", err)
		return 1
	}
	printCompileWarnings(stderr, warnings)
	fmt.Fprintf(stderr, "compiled: %d entries\n", count)
	return 0
}

func resolveVault() (vault.Vault, error) {
	wd, err := os.Getwd()
	if err != nil {
		return vault.Vault{}, fmt.Errorf("get current directory: %w", err)
	}
	return vault.Discover(wd)
}
