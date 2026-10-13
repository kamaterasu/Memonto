package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCheckAnswer(t *testing.T) {
	c := Card{Answer: "--force"}
	for ans, want := range map[string]bool{"--force": true, " --force ": true, "-": false, "f": false, "": false, "--FORCE": false} {
		if got := checkAnswer(c, ans); got != want {
			t.Errorf("checkAnswer(%q) = %v, want %v", ans, got, want)
		}
	}
}

func TestEnterGradesOnce(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := initialModel([]Card{{ID: "a", Answer: "--force", Box: 1, NextDue: time.Now().Add(-time.Hour)}})
	m.input.SetValue("--force")
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	next, _ := m.Update(enter)
	next, _ = next.Update(enter)
	if box := next.(model).cards[0].Box; box != 2 {
		t.Errorf("box = %d after one correct answer, want 2", box)
	}
}

func TestDataDirMigratesOldName(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	if err := os.MkdirAll(filepath.Join(base, "memento"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "memento", "cards.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := cardsPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil || p != filepath.Join(base, "memonto", "cards.json") {
		t.Errorf("cards not migrated: %s %v", p, err)
	}
}

func TestFeedbackShowsFullCommand(t *testing.T) {
	c := Card{Answer: "--amend", Command: "git commit --amend --no-edit"}
	for _, ok := range []bool{true, false} {
		if fb := feedbackLine(ok, c); !strings.Contains(fb, c.Command) {
			t.Errorf("feedbackLine(%v) = %q", ok, fb)
		}
	}
}
