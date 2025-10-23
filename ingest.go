package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type CommandEvent struct {
	When    time.Time
	Command string
}

var (
	pathLike   = regexp.MustCompile(`(~|\.{1,2}|/)[\w@./\-+:%]+`)
	urlRe      = regexp.MustCompile(`https?://\S+`)
	uuidRe     = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	shaRe      = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	ipRe       = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	bigNumRe   = regexp.MustCompile(`\b\d{3,}\b`)
	varAssign  = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*=([^ \t]+)`)
	wsCollapse = regexp.MustCompile(`\s+`)
)

var valueFlags = map[string]string{
	"-o": "<PATH>", "--output": "<PATH>", "-i": "<PATH>", "--input": "<PATH>",
	"-f": "<FILE>", "--file": "<FILE>", "--namespace": "<NS>", "-n": "<NS>",
	"--context": "<CTX>", "-r": "<REPO>", "--repo": "<REPO>",
	"--kubeconfig": "<PATH>", "--config": "<PATH>",
}

func ParseHistory() []CommandEvent {
	uniq := make(map[string]CommandEvent)
	paths := guessHistoryFiles()

	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			raw, when := normalizeHistoryLine(line)
			raw = scrub(raw)
			if isIgnorable(raw) {
				continue
			}
			canon := normalizeCommand(raw)

			prev, ok := uniq[canon]
			if !ok || when.After(prev.When) {
				uniq[canon] = CommandEvent{When: when, Command: canon}
			}
		}
		_ = f.Close()
	}

	events := make([]CommandEvent, 0, len(uniq))
	for _, ev := range uniq {
		events = append(events, ev)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].When.After(events[j].When) })
	return events
}

func guessHistoryFiles() []string {
	h, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(h, ".zsh_history"),
		filepath.Join(h, ".bash_history"),
	}
	out := []string{}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

var zshExt = regexp.MustCompile(`^: (\d+):(\d+);`)

func normalizeHistoryLine(line string) (cmd string, when time.Time) {
	if m := zshExt.FindStringSubmatch(line); len(m) == 3 {
		// Zsh extended history
		epoch := m[1]
		// strip prefix
		cmd = strings.TrimSpace(strings.TrimPrefix(line, m[0]))
		sec, _ := time.ParseDuration(epoch + "s")
		when = time.Unix(0, 0).Add(sec)
		return cmd, when
	}
	// Bash: just the command; no timestamp
	return line, time.Time{}
}

// Scrub obvious secrets and emails.
var (
	emailRe   = regexp.MustCompile(`\b[\w._%+-]+@[\w.-]+\.[A-Za-z]{2,}\b`)
	hexRe     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	tokenRe   = regexp.MustCompile(`(?i)(AWS|SECRET|TOKEN|KEY|PASSWORD|PASS|PWD)=\S+`)
	quoteBlob = regexp.MustCompile(`'[^']+'|"[^"]+"`)
)

func scrub(s string) string {
	s = tokenRe.ReplaceAllString(s, "$1=***")
	s = emailRe.ReplaceAllString(s, "***@***")
	s = hexRe.ReplaceAllString(s, "<HEX>")
	return s
}

func isIgnorable(s string) bool {
	if strings.HasPrefix(s, "#") {
		return true
	}
	if strings.HasPrefix(s, "cd ") {
		return true
	}
	if strings.HasPrefix(s, "ls") {
		return true
	}
	if len(strings.Fields(s)) == 0 {
		return true
	}
	return false
}

// Heuristic: mark as tricky if it's long, has pipes, multiple flags, or risky flags.
func isTricky(cmd string) bool {
	flags := strings.Count(cmd, " -") + strings.Count(cmd, " --")
	return len(cmd) > 40 || strings.Contains(cmd, "|") || strings.Contains(cmd, "&&") || flags >= 2 ||
		strings.Contains(cmd, "-rf") || strings.Contains(cmd, "--force")
}

func GenerateCards(events []CommandEvent, existing []Card) []Card {
	idx := map[string]*Card{}
	for i := range existing {
		idx[existing[i].ID] = &existing[i]
	}

	out := []Card{}
	seenIDs := make(map[string]bool)

	for _, ev := range events {
		if !isTricky(ev.Command) {
			continue
		}

		canon := normalizeCommand(ev.Command)
		id := hash(canon)

		if seenIDs[id] {
			continue
		}
		if c, ok := idx[id]; ok {
			c.SeenCount++
			continue
		}

		prompt, answer, hint := cloze(canon)
		out = append(out, Card{
			ID: id, Prompt: prompt, Answer: answer, Hint: hint, Command: canon,
			Tags: deriveTags(canon), Box: 1, NextDue: time.Now(), SeenCount: 1,
		})
		seenIDs[id] = true
	}
	return out
}

func deriveTags(cmd string) []string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	cmdName := parts[0]
	tags := []string{cmdName}
	for _, t := range []string{"git", "kubectl", "ffmpeg", "docker", "grep", "awk", "sed"} {
		if strings.HasPrefix(cmd, t+" ") {
			tags = append(tags, t)
		}
	}
	return unique(tags)
}
func unique(ss []string) []string {
	m := map[string]struct{}{}
	out := []string{}
	for _, s := range ss {
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func hash(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

func normalizeCommand(s string) string {
	// strip/standardize quotes first
	s = quoteBlob.ReplaceAllString(s, "<STR>")

	// mask volatile atoms
	s = urlRe.ReplaceAllString(s, "<URL>")
	s = emailRe.ReplaceAllString(s, "***@***")
	s = uuidRe.ReplaceAllString(s, "<UUID>")
	s = shaRe.ReplaceAllString(s, "<SHA>")
	s = ipRe.ReplaceAllString(s, "<IP>")
	s = bigNumRe.ReplaceAllString(s, "<NUM>")
	s = varAssign.ReplaceAllString(s, "${VAR}=<VAL>")
	s = pathLike.ReplaceAllString(s, "<PATH>")

	// token-level pass to replace values after known flags
	toks := strings.Fields(s)
	for i := 0; i < len(toks); i++ {
		if ph, ok := valueFlags[toks[i]]; ok && i+1 < len(toks) {
			// don't stomp other flags
			if !strings.HasPrefix(toks[i+1], "-") {
				toks[i+1] = ph
			}
		}
	}

	// optional: sort standalone long flags for stability (mostly safe)
	toks = stableFlagOrder(toks)

	// rebuild & collapse whitespace
	out := strings.Join(toks, " ")
	out = wsCollapse.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}

func stableFlagOrder(toks []string) []string {
	// move --long-flags that don’t have attached values into a stable order
	flags, rest := []string{}, []string{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if strings.HasPrefix(t, "--") {
			// if next token is a value (not a flag), keep pair in rest
			if i+1 < len(toks) && !strings.HasPrefix(toks[i+1], "-") && valueFlags[t] != "" {
				rest = append(rest, t, toks[i+1])
				i++
			} else {
				flags = append(flags, t)
			}
		} else {
			rest = append(rest, t)
		}
	}
	sort.Strings(flags)
	return append(append([]string{}, rest[0:1]...), append(flags, rest[1:]...)...)
}

func isBadAnswerToken(w string) bool {
	if w == "" {
		return true
	}
	if strings.Contains(w, "<") && strings.Contains(w, ">") {
		return true
	} // placeholders
	if strings.Contains(w, "/") || strings.HasPrefix(w, "~") || strings.HasPrefix(w, ".") {
		return true
	}
	if urlRe.MatchString(w) || pathLike.MatchString(w) || shaRe.MatchString(w) || uuidRe.MatchString(w) {
		return true
	}
	if bigNumRe.MatchString(w) {
		return true
	}
	return false
}

func preferSubcommands(cmdName string) map[string]bool {
	switch cmdName {
	case "git":
		return set("rebase", "cherry-pick", "stash", "reset", "restore", "revert", "checkout", "commit", "fetch", "merge", "push", "pull")
	case "kubectl":
		return set("get", "describe", "apply", "delete", "logs", "exec", "rollout", "scale", "port-forward", "top")
	case "ffmpeg":
		return set() // mostly flags
	default:
		return set()
	}
}

func set(ss ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func cloze(cmd string) (prompt, answer, hint string) {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return "", "", ""
	}

	candidates := []int{}
	// 1) explicit “good” tokens
	good := preferSubcommands(words[0])
	for i := 1; i < len(words); i++ {
		if good[words[i]] {
			candidates = append(candidates, i)
		}
	}
	// 2) long flags
	for i := 0; i < len(words); i++ {
		if strings.HasPrefix(words[i], "--") {
			candidates = append(candidates, i)
		}
	}
	// 3) short flags
	for i := 0; i < len(words); i++ {
		if strings.HasPrefix(words[i], "-") && !strings.HasPrefix(words[i], "--") {
			candidates = append(candidates, i)
		}
	}
	// 4) fallback: the first non-dynamic non-command token
	if len(candidates) == 0 {
		for i := 1; i < len(words); i++ {
			if !isBadAnswerToken(words[i]) {
				candidates = append(candidates, i)
				break
			}
		}
	}

	// pick first candidate that isn’t junk
	idx := -1
	for _, i := range candidates {
		if !isBadAnswerToken(words[i]) {
			idx = i
			break
		}
	}
	if idx == -1 {
		idx = 0
	} // final fallback (rare)

	answer = words[idx]
	masked := append([]string{}, words...)
	masked[idx] = "_____"
	prompt = strings.Join(masked, " ")
	hint = "Type the missing flag/subcommand"
	return
}
