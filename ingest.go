package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CommandEvent struct {
	When    time.Time
	Command string // scrubbed
}

func ParseHistory() []CommandEvent {
	var events []CommandEvent
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
			cmd, when := normalizeHistoryLine(line)
			cmd = scrub(cmd)
			if isIgnorable(cmd) {
				continue
			}
			events = append(events, CommandEvent{When: when, Command: cmd})
		}
		_ = f.Close()
	}
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

var zshExt = regexp.MustCompile(`^: (\d+):(\d+);`) // : <epoch>:<dur>;cmd

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
	seen := map[string]bool{}
	for _, c := range existing {
		seen[c.ID] = true
	}
	var out []Card
	for _, ev := range events {
		if !isTricky(ev.Command) {
			continue
		}
		prompt, answer, hint := cloze(ev.Command)
		id := hash(normalizeCommand(ev.Command))
		if seen[id] {
			continue
		}
		tags := deriveTags(ev.Command)
		out = append(out, Card{
			ID: id, Prompt: prompt, Answer: answer, Hint: hint, Command: ev.Command,
			Tags: tags, Box: 1, NextDue: time.Now(),
		})
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

// normalizeCommand removes volatile args (paths, quoted blobs) for ID hashing.
func normalizeCommand(s string) string {
	s = quoteBlob.ReplaceAllString(s, "<STR>")
	s = regexp.MustCompile(`/[^\s]+`).ReplaceAllString(s, "/<PATH>")
	return s
}

// cloze hides an interesting flag or subcommand to create a recall prompt.
func cloze(cmd string) (prompt, answer, hint string) {
	// Prefer hiding long flags, then short flags, else subcommand word.
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return "", "", ""
	}
	// Search candidate index
	idx := -1
	for i, w := range words {
		if strings.HasPrefix(w, "--") {
			idx = i
			break
		}
	}
	if idx == -1 {
		for i, w := range words {
			if strings.HasPrefix(w, "-") {
				idx = i
				break
			}
		}
	}
	if idx == -1 && len(words) > 1 {
		idx = 1
	} // subcommand
	if idx == -1 {
		idx = 0
	}

	answer = words[idx]
	masked := make([]string, len(words))
	copy(masked, words)
	masked[idx] = "_____"
	prompt = strings.Join(masked, " ")
	hint = "Type the missing flag/word"
	return
}
