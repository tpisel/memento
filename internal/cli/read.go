package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/markdown"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/vault"
)

const readNoManifestLinkSurfaceHint = "note: no manifest; link surface unavailable. run: memento compile"

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
		printReadError(stderr, v, target, err)
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

	linkManifest, ok, err := manifestForReadLinks(v, target, numberedManifest)
	if err != nil {
		printCLIError(stderr, "read", err)
		return 1
	}
	fmt.Fprintf(stderr, "binding: %s\n", binding)
	if ok {
		if entry, entryOK := manifestEntryByKey(linkManifest, readKey); entryOK {
			fmt.Fprintf(stderr, "summary: %s\n", entry.SummaryState)
		}
		var lines []string
		if _, section, hasSection := strings.Cut(target, "#"); hasSection {
			lines = readSectionLinkSurfaceLines(linkManifest, readKey, section, data, sectionInlinksAnchorFiltered)
		} else {
			lines = readLinkSurfaceLines(linkManifest, readKey)
		}
		for _, line := range lines {
			fmt.Fprintln(stderr, line)
		}
	} else {
		fmt.Fprintln(stderr, readNoManifestLinkSurfaceHint)
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

func printReadError(stderr io.Writer, v vault.Vault, target string, err error) {
	if errors.Is(err, note.ErrNotFound) && !strings.HasPrefix(target, "@") {
		if suggestions := readNotFoundSuggestions(v, target); len(suggestions) > 0 {
			token, hint := errorToken(err), errorHint(err)
			fmt.Fprintf(stderr, "memento read: %s: %v — did you mean: %s?\n", token, err, strings.Join(suggestions, ", "))
			if hint != "" {
				fmt.Fprintln(stderr, hint)
			}
			return
		}
	}
	printCLIError(stderr, "read", err)
}

func readNotFoundSuggestions(v vault.Vault, target string) []string {
	requestedKey, _, _ := strings.Cut(target, "#")
	m, err := readManifest(v)
	if err != nil {
		return nil
	}

	suggestions := []string{}
	seen := map[string]bool{}
	for _, numbered := range brief.NumberedEntries(m) {
		key := numbered.Entry.Key
		if seen[key] || !entryKeyMatchesReadSuggestion(key, requestedKey) {
			continue
		}
		if _, err := os.Stat(filepath.Join(v.Root, filepath.FromSlash(key))); err != nil {
			continue
		}
		seen[key] = true
		suggestions = append(suggestions, fmt.Sprintf("%s (@%d)", key, numbered.Number))
		if len(suggestions) == 3 {
			break
		}
	}
	return suggestions
}

func entryKeyMatchesReadSuggestion(entryKey, requestedKey string) bool {
	base := path.Base(entryKey)
	stem := strings.TrimSuffix(base, ".md")
	return base == requestedKey ||
		stem == requestedKey ||
		strings.EqualFold(base, requestedKey) ||
		strings.EqualFold(stem, requestedKey)
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
	if entries := linkSurfaceInEntries(entry.Links.In, numberByKey, markdown.WikiLinkTypeWiki); len(entries) > 0 {
		lines = append(lines, "inlinks: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceOutEntries(entry.Links.Out, numberByKey, markdown.WikiLinkTypeWiki); len(entries) > 0 {
		lines = append(lines, "outlinks: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceOutEntries(entry.Links.Out, numberByKey, markdown.WikiLinkTypeEmbed); len(entries) > 0 {
		lines = append(lines, "transcludes: "+strings.Join(entries, ", "))
	}
	if entries := linkSurfaceInEntries(entry.Links.In, numberByKey, markdown.WikiLinkTypeEmbed); len(entries) > 0 {
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

func linkSurfaceOutEntries(links []manifest.OutLink, numberByKey map[string]int, linkType markdown.WikiLinkType) []string {
	entries := []string{}
	for _, link := range links {
		if link.Type != linkType {
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

func linkSurfaceInEntries(links []manifest.InLink, numberByKey map[string]int, linkType markdown.WikiLinkType) []string {
	entries := []string{}
	for _, link := range links {
		if link.Type != linkType {
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
	numberTarget, section, hasSection := strings.Cut(target, "#")
	number, err := strconv.Atoi(numberTarget)
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
	readTarget := key
	if hasSection {
		readTarget += "#" + section
	}
	data, err := note.Read(v, readTarget)
	if err != nil {
		if errors.Is(err, note.ErrNotFound) {
			return nil, "", manifest.Manifest{}, fmt.Errorf("%w: entry %d's file `%s` no longer exists", manifest.ErrStale, number, key)
		}
		return nil, "", manifest.Manifest{}, err
	}

	return data, readTarget, m, nil
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
