package main

import (
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
	Count   int
}

var (
	pathLike  = regexp.MustCompile(`(^|[\s=])(?:~|\.{1,2})?/[\w@./\-+:%]*`)
	urlRe     = regexp.MustCompile(`https?://\S+`)
	uuidRe    = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	shaRe     = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	ipRe      = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	bigNumRe  = regexp.MustCompile(`\b\d{3,}\b`)
	varAssign = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)=[^ \t]+`)
)

var longValueFlags = map[string]string{
	"--output": "<PATH>", "--input": "<PATH>", "--file": "<FILE>", "--namespace": "<NS>",
	"--context": "<CTX>", "--repo": "<REPO>", "--kubeconfig": "<PATH>", "--config": "<PATH>",
}

// Short flags mean different things per tool (grep -i vs ffmpeg -i), so scope them.
var shortValueFlags = map[string]map[string]string{
	"kubectl": {"-n": "<NS>", "-f": "<FILE>"},
	"helm":    {"-n": "<NS>", "-f": "<FILE>"},
	"docker":  {"-f": "<FILE>"},
	"ffmpeg":  {"-i": "<PATH>"},
	"curl":    {"-o": "<PATH>"},
	"gh":      {"-R": "<REPO>"},
}

var cmdSeparators = map[string]bool{"|": true, "||": true, "&&": true, ";": true}

func ParseHistory() ([]CommandEvent, error) {
	uniq := make(map[string]CommandEvent)

	for _, p := range guessHistoryFiles() {
		lines, err := readHistoryFile(p)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			raw, when := normalizeHistoryLine(line)
			raw = scrub(raw)
			if isIgnorable(raw) {
				continue
			}
			canon := normalizeCommand(raw)

			ev := uniq[canon]
			ev.Command = canon
			ev.Count++
			if when.After(ev.When) {
				ev.When = when
			}
			uniq[canon] = ev
		}
	}

	events := make([]CommandEvent, 0, len(uniq))
	for _, ev := range uniq {
		events = append(events, ev)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].When.After(events[j].When) })
	return events, nil
}

// readHistoryFile returns one entry per command, joining backslash-continued lines.
func readHistoryFile(p string) ([]string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if strings.Contains(filepath.Base(p), "zsh") {
		b = unmetafy(b)
	}
	out := []string{}
	cur := ""
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.HasSuffix(l, "\\") {
			cur += strings.TrimSuffix(l, "\\") + " "
			continue
		}
		if line := strings.TrimSpace(cur + l); line != "" {
			out = append(out, line)
		}
		cur = ""
	}
	if line := strings.TrimSpace(cur); line != "" {
		out = append(out, line)
	}
	return out, nil
}

// zsh stores non-ASCII bytes as 0x83 followed by byte^0x20.
func unmetafy(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == 0x83 && i+1 < len(b) {
			i++
			out = append(out, b[i]^0x20)
			continue
		}
		out = append(out, b[i])
	}
	return out
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

	urlCredRe    = regexp.MustCompile(`(://)[^/\s:@]+:[^/\s@]+@`)
	secretFlagRe = regexp.MustCompile(`(?i)(--?(?:password|passwd|pass|token|secret|api-?key)[= ])[^\s'"]+`)
	userPassRe   = regexp.MustCompile(`(\s(?:-u|--user)[= ]?)[^\s'"]+:[^\s'"]+`)
	mysqlPassRe  = regexp.MustCompile(`(\b(?:mysql|mysqldump|mariadb)\b.*?\s-p)[^\s'"]+`)
	bearerRe     = regexp.MustCompile(`(?i)(bearer\s+)[^\s'"]+`)
	knownTokenRe = regexp.MustCompile(`\b(?:gh[pousr]_|github_pat_|glpat-|xox[abprs]-|sk-|AKIA)[A-Za-z0-9_\-]{10,}`)
)

func scrub(s string) string {
	s = urlCredRe.ReplaceAllString(s, "${1}***@")
	s = secretFlagRe.ReplaceAllString(s, "${1}***")
	s = userPassRe.ReplaceAllString(s, "${1}***")
	s = mysqlPassRe.ReplaceAllString(s, "${1}***")
	s = bearerRe.ReplaceAllString(s, "${1}***")
	s = knownTokenRe.ReplaceAllString(s, "<SECRET>")
	s = tokenRe.ReplaceAllString(s, "$1=***")
	s = emailRe.ReplaceAllString(s, "***@***")
	s = hexRe.ReplaceAllString(s, "<HEX>")
	return s
}

func isIgnorable(s string) bool {
	f := strings.Fields(s)
	return len(f) == 0 || strings.HasPrefix(f[0], "#") || ignoredHeads[f[0]]
}

// shell keywords: loop/if one-liners make poor cards since the blank belongs to an inner tool
var ignoredHeads = map[string]bool{"cd": true, "ls": true, "for": true, "while": true, "until": true, "if": true}

// Heuristic: mark as tricky if it has pipes, multiple flags, or risky flags.
func isTricky(cmd string) bool {
	flags := strings.Count(cmd, " -")
	return strings.Contains(cmd, "|") || strings.Contains(cmd, "&&") || flags >= 2 ||
		strings.Contains(cmd, "-rf") || strings.Contains(cmd, "--force")
}

// toolIndex skips leading sudo and VAR=value assignments to find the real tool.
func toolIndex(words []string) int {
	for i, w := range words {
		if w != "sudo" && !strings.Contains(w, "=") {
			return i
		}
	}
	return 0
}

// clozeKey identifies what a card teaches; one card per tool+answer is enough.
func clozeKey(cmd, answer string) string {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return answer
	}
	return words[toolIndex(words)] + " " + answer
}

func GenerateCards(events []CommandEvent, existing []Card) []Card {
	idx := map[string]*Card{}
	taken := map[string]bool{}
	for i := range existing {
		idx[existing[i].ID] = &existing[i]
		taken[clozeKey(existing[i].Command, existing[i].Answer)] = true
	}

	// most-used first, so the variant you actually type wins the tool+answer slot
	sorted := append([]CommandEvent{}, events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })

	out := []Card{}
	for _, ev := range sorted {
		if !isTricky(ev.Command) {
			continue
		}
		id := hash(ev.Command)
		if c, ok := idx[id]; ok {
			c.HistoryCount = ev.Count
			continue
		}

		prompt, answer, hint := cloze(ev.Command)
		key := clozeKey(ev.Command, answer)
		if answer == "" || taken[key] {
			continue
		}
		taken[key] = true
		out = append(out, Card{
			ID: id, Prompt: prompt, Answer: answer, Hint: hint, Command: ev.Command,
			Tags: deriveTags(ev.Command), Box: 1, NextDue: time.Now(), HistoryCount: ev.Count,
		})
	}
	return out
}

func deriveTags(cmd string) []string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	cmdName := parts[toolIndex(parts)]
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
	s = varAssign.ReplaceAllString(s, "${1}=<VAL>")
	s = pathLike.ReplaceAllString(s, "${1}<PATH>")

	// token-level pass to replace values after known flags, tracking the tool per pipeline segment
	toks := strings.Fields(s)
	tool := ""
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if cmdSeparators[t] {
			tool = ""
			continue
		}
		if tool == "" {
			if t != "sudo" && !strings.Contains(t, "=") {
				tool = t
			}
			continue
		}
		ph, ok := longValueFlags[t]
		if !ok {
			ph, ok = shortValueFlags[tool][t]
		}
		// don't stomp other flags
		if ok && i+1 < len(toks) && !strings.HasPrefix(toks[i+1], "-") && !cmdSeparators[toks[i+1]] {
			toks[i+1] = ph
		}
	}
	return strings.Join(toks, " ")
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

	t := toolIndex(words)
	candidates := []int{}
	// 1) explicit “good” tokens
	good := preferSubcommands(words[t])
	for i := t + 1; i < len(words); i++ {
		if good[words[i]] {
			candidates = append(candidates, i)
		}
	}
	// 2) long flags
	for i := t + 1; i < len(words); i++ {
		if strings.HasPrefix(words[i], "--") {
			candidates = append(candidates, i)
		}
	}
	// 3) short flags
	for i := t + 1; i < len(words); i++ {
		if strings.HasPrefix(words[i], "-") && !strings.HasPrefix(words[i], "--") {
			candidates = append(candidates, i)
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
		return "", "", ""
	}

	answer = words[idx]
	masked := append([]string{}, words...)
	masked[idx] = "_____"
	prompt = strings.Join(masked, " ")
	hint = "Type the missing flag/subcommand"
	return
}
