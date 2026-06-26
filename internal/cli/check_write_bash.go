package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/enforce"
	"github.com/tpisel/memento/internal/vault"
)

// reasonBashOpaqueWrite is the denial code for a Bash command that resolves to a
// vault path but is not the one provably-append shape (ADR-0031): anything we can
// see writing the vault that is not a single, standalone `>>` redirect.
const reasonBashOpaqueWrite = "bash_opaque_write"

// bashOpaqueMessage is the productive-wall text for a denied Bash vault write. It
// has no <key> because the denial fires precisely when the target is not a single
// clean append we can name; the recovery is to use a tool memento can derive.
const bashOpaqueMessage = "This Bash command writes into the memento vault in a way memento cannot prove is a safe append, " +
	"so it is denied and the identical command will be denied again. " +
	"Use the Write or Edit tool, or a single standalone `>> <vault-file>` append, so memento can check the note's mode."

// bashVerdict is the outcome of classifying a Bash command against the vault.
type bashVerdict int

const (
	// bashInert: no reference the parser recognises resolves to a vault path, so
	// the command is left to normal permission flow. This is the documented
	// fail-open boundary — vars, $(...), `…`, eval, and interpreter -c hide their
	// targets from a static parser, so a write through them lands here, not in a
	// deny (ADR-0031: broad-deny over *recognisable* references only).
	bashInert bashVerdict = iota
	// bashAppend: a single command segment whose only vault reference is a literal
	// `>>` redirect — the one provably-append shape, then mode-gated.
	bashAppend
	// bashOpaque: a recognisable reference resolves to a vault path but is not that
	// shape (a truncating `>`, a second redirect, a compound, a known mutator) —
	// denied as bash_opaque_write.
	bashOpaque
)

// checkWriteBash gives the verdict for a Bash command (ADR-0031, "deny unless
// provably append"). The only allowed vault write is a single standalone `>>`
// redirect; that proven append is mode-gated exactly like a file write (a `>>`
// always keeps the old bytes as a prefix, so append-only and living allow and
// read-only denies). Everything else recognisably touching the vault is denied;
// references the parser cannot see fall through to normal permission flow.
func checkWriteBash(command string, stdout, stderr io.Writer) int {
	v, ok, code := resolveVaultForCheck(stdout, stderr, "this Bash command's target")
	if !ok {
		return code
	}

	verdict, key := classifyBash(v, command)
	switch verdict {
	case bashOpaque:
		emitVerdict(stdout, "deny", reasonBashOpaqueWrite, bashOpaqueMessage)
		return 0
	case bashAppend:
		// A `>>` is, by construction, an append: model new-bytes as the old bytes
		// plus a non-empty suffix so the shared gate's prefix invariant allows it
		// on append-only/living and denies it on read-only (where we cannot prove
		// the appended bytes are empty), reusing the file-write messages.
		return gateVaultWrite(v, key, "Bash", enforce.ReasonAppendOnlyOverwrite, stdout, stderr,
			func(old []byte, _ bool) ([]byte, error) {
				return append(append([]byte{}, old...), '\n'), nil
			})
	default:
		return 0
	}
}

// classifyBash parses command and decides which bucket it falls into, returning
// the vault key for the bashAppend case. It collects every reference the parser
// recognises (redirections and known mutators) that resolves to a vault path; the
// narrow allow is exactly one such reference, a `>>` redirect, in a single
// command segment. Zero references is inert; anything else is opaque.
func classifyBash(v vault.Vault, command string) (bashVerdict, string) {
	segs := splitBashSegments(tokenizeBash(command))

	type vaultRef struct {
		appendRedir bool
		key         string
	}
	var refs []vaultRef

	for _, seg := range segs {
		for _, r := range seg.redirs {
			if r.op == "<" || r.op == "<<" {
				continue // input redirection reads, it does not write
			}
			if key, ok := vaultKeyOf(v, r.operand); ok {
				refs = append(refs, vaultRef{appendRedir: r.op == ">>", key: key})
			}
		}
		for _, key := range mutatorVaultTargets(v, seg.words) {
			refs = append(refs, vaultRef{key: key})
		}
	}

	switch {
	case len(refs) == 0:
		return bashInert, ""
	case len(refs) == 1 && refs[0].appendRedir && len(segs) == 1:
		return bashAppend, refs[0].key
	default:
		return bashOpaque, ""
	}
}

// vaultKeyOf resolves a literal command operand to a vault-relative key, reporting
// whether it lands in the vault. An operand the parser flagged opaque (a variable,
// command substitution, or empty) is never recognised as a vault reference.
func vaultKeyOf(v vault.Vault, operand bashTok) (string, bool) {
	if operand.opaque || operand.word == "" {
		return "", false
	}
	key, inVault, err := vaultRelativeKey(v, expandTilde(operand.word))
	if err != nil || !inVault {
		return "", false
	}
	return key, true
}

// mutatorVaultTargets returns the vault keys a known in-place/destination mutator
// in seg would write. The set mirrors the legacy guard (tee, cp, mv, install, sed
// -i, perl -i, dd of=); a mutator outside it is invisible here and falls through
// to fail-open, a documented guardrail limit rather than a gap to close with a
// bigger parser.
func mutatorVaultTargets(v vault.Vault, words []bashTok) []string {
	i := 0
	for i < len(words) && isAssignment(words[i].word) {
		i++ // skip VAR=val command prefixes to reach the command name
	}
	if i >= len(words) {
		return nil
	}
	name := filepath.Base(words[i].word)
	args := words[i+1:]

	var targets []string
	add := func(t bashTok) {
		if key, ok := vaultKeyOf(v, t); ok {
			targets = append(targets, key)
		}
	}

	switch name {
	case "tee":
		for _, a := range optionless(args) {
			add(a)
		}
	case "cp", "mv", "install":
		if ops := optionless(args); len(ops) >= 2 {
			add(ops[len(ops)-1])
		}
	case "sed":
		if hasSedInplace(args) {
			for _, a := range optionless(args) {
				add(a)
			}
		}
	case "perl":
		if hasPerlInplace(args) {
			for _, a := range optionless(args) {
				add(a)
			}
		}
	case "dd":
		for _, a := range args {
			if !a.opaque && strings.HasPrefix(a.word, "of=") {
				add(bashTok{word: strings.TrimPrefix(a.word, "of=")})
			}
		}
	}
	return targets
}

// optionless drops option-looking args (a leading '-', other than a bare '-'),
// stopping option parsing after a '--', leaving operands a mutator acts on.
func optionless(args []bashTok) []bashTok {
	var out []bashTok
	afterOptions := false
	for _, a := range args {
		if !afterOptions && a.word == "--" {
			afterOptions = true
			continue
		}
		if !afterOptions && strings.HasPrefix(a.word, "-") && a.word != "-" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func hasSedInplace(args []bashTok) bool {
	for _, a := range args {
		if a.word == "-i" || strings.HasPrefix(a.word, "-i") {
			return true
		}
	}
	return false
}

func hasPerlInplace(args []bashTok) bool {
	var flags strings.Builder
	for _, a := range args {
		if strings.HasPrefix(a.word, "-") && a.word != "--" {
			flags.WriteString(a.word[1:])
		}
	}
	f := flags.String()
	return strings.Contains(f, "p") && strings.Contains(f, "i")
}

// isAssignment reports whether w is a NAME=value shell assignment prefix.
func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := w[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// expandTilde resolves a leading ~ or ~/ to the user's home directory, matching
// the shell's own expansion before path resolution.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// bashTok is one token of a parsed command: a word (op == "") or an operator
// (word == ""). opaque marks a word that contains a shell expansion the parser
// cannot resolve, so it must never be treated as a literal path.
type bashTok struct {
	word   string
	op     string
	opaque bool
}

// tokenizeBash splits a command into words and the operators that matter for the
// classifier (redirections and segment boundaries). It is deliberately partial:
// it tracks quotes and escapes well enough to find literal redirect targets, and
// flags variables / command substitution as opaque so they are never resolved as
// paths. It is a guardrail parser, not a shell.
func tokenizeBash(cmd string) []bashTok {
	var toks []bashTok
	var buf strings.Builder
	started, opaque := false, false

	flush := func() {
		if started {
			toks = append(toks, bashTok{word: buf.String(), opaque: opaque})
			buf.Reset()
			started, opaque = false, false
		}
	}
	op := func(s string) { toks = append(toks, bashTok{op: s}) }
	// redirOp flushes a pending word — unless it is a bare fd number glued to the
	// operator (2>file) — then emits the redirection operator.
	redirOp := func(s string) {
		if started && isAllDigits(buf.String()) {
			buf.Reset()
			started, opaque = false, false
		} else {
			flush()
		}
		op(s)
	}

	n := len(cmd)
	for i := 0; i < n; {
		c := cmd[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			flush()
			i++
		case c == '\n':
			flush()
			op(";")
			i++
		case c == '\\':
			started = true
			if i+1 < n {
				buf.WriteByte(cmd[i+1])
				i += 2
			} else {
				i++
			}
		case c == '\'':
			started = true
			if j := strings.IndexByte(cmd[i+1:], '\''); j < 0 {
				buf.WriteString(cmd[i+1:])
				i = n
			} else {
				buf.WriteString(cmd[i+1 : i+1+j])
				i = i + 1 + j + 1
			}
		case c == '"':
			started = true
			i++
			for i < n && cmd[i] != '"' {
				if cmd[i] == '\\' && i+1 < n {
					if next := cmd[i+1]; next == '"' || next == '\\' || next == '$' || next == '`' {
						buf.WriteByte(next)
						i += 2
						continue
					}
				}
				if cmd[i] == '$' || cmd[i] == '`' {
					opaque = true
				}
				buf.WriteByte(cmd[i])
				i++
			}
			if i < n {
				i++ // closing quote
			}
		case c == '`':
			started, opaque = true, true
			if j := strings.IndexByte(cmd[i+1:], '`'); j < 0 {
				i = n
			} else {
				i = i + 1 + j + 1
			}
		case c == '$':
			started, opaque = true, true
			switch {
			case i+1 < n && cmd[i+1] == '(':
				i = skipBalanced(cmd, i+1, '(', ')')
			case i+1 < n && cmd[i+1] == '{':
				if j := strings.IndexByte(cmd[i+1:], '}'); j < 0 {
					i = n
				} else {
					i = i + 1 + j + 1
				}
			default:
				buf.WriteByte('$')
				i++
			}
		case c == ';':
			flush()
			if i+1 < n && cmd[i+1] == ';' {
				op(";;")
				i += 2
			} else {
				op(";")
				i++
			}
		case c == '&':
			flush()
			switch {
			case i+1 < n && cmd[i+1] == '&':
				op("&&")
				i += 2
			case i+2 < n && cmd[i+1] == '>' && cmd[i+2] == '>':
				op("&>>")
				i += 3
			case i+1 < n && cmd[i+1] == '>':
				op("&>")
				i += 2
			default:
				op("&")
				i++
			}
		case c == '|':
			flush()
			switch {
			case i+1 < n && cmd[i+1] == '|':
				op("||")
				i += 2
			case i+1 < n && cmd[i+1] == '&':
				op("|&")
				i += 2
			default:
				op("|")
				i++
			}
		case c == '(':
			flush()
			op("(")
			i++
		case c == ')':
			flush()
			op(")")
			i++
		case c == '>':
			if i+1 < n && cmd[i+1] == '>' {
				redirOp(">>")
				i += 2
			} else if i+1 < n && cmd[i+1] == '&' {
				redirOp(">&")
				i += 2
			} else {
				redirOp(">")
				i++
			}
		case c == '<':
			if i+1 < n && cmd[i+1] == '<' {
				redirOp("<<")
				i += 2
			} else {
				redirOp("<")
				i++
			}
		default:
			started = true
			buf.WriteByte(c)
			i++
		}
	}
	flush()
	return toks
}

// bashRedir is a redirection operator paired with its operand token.
type bashRedir struct {
	op      string
	operand bashTok
}

// bashSeg is one command segment: its command words and its redirections, split
// out from the surrounding pipeline / list operators.
type bashSeg struct {
	words  []bashTok
	redirs []bashRedir
}

// splitBashSegments groups a token stream into command segments, breaking on
// pipeline and list operators and subshell parentheses, and binding each
// redirection operator to the operand that follows it.
func splitBashSegments(toks []bashTok) []bashSeg {
	boundaries := map[string]bool{
		";": true, ";;": true, "&&": true, "||": true, "|": true, "|&": true, "&": true, "(": true, ")": true,
	}
	redirs := map[string]bool{">": true, ">>": true, ">&": true, "&>": true, "&>>": true, "<": true, "<<": true}

	var segs []bashSeg
	cur := bashSeg{}
	push := func() {
		if len(cur.words) > 0 || len(cur.redirs) > 0 {
			segs = append(segs, cur)
		}
		cur = bashSeg{}
	}

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.op == "":
			cur.words = append(cur.words, t)
		case boundaries[t.op]:
			push()
		case redirs[t.op]:
			var operand bashTok
			if i+1 < len(toks) && toks[i+1].op == "" {
				operand = toks[i+1]
				i++
			}
			cur.redirs = append(cur.redirs, bashRedir{op: t.op, operand: operand})
		}
	}
	push()
	return segs
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// skipBalanced returns the index just past the operator that closes the open
// bracket at s[open], handling nesting; on an unterminated bracket it returns
// len(s).
func skipBalanced(s string, open int, openCh, closeCh byte) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}
