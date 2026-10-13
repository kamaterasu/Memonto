package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeCommand(t *testing.T) {
	cases := map[string]string{
		"git commit --amend --no-edit":         "git commit --amend --no-edit",
		"git log --oneline | grep --color fix": "git log --oneline | grep --color fix",
		"FOO=bar make build":                   "FOO=<VAL> make build",
		"grep -i foo file.txt":                 "grep -i foo file.txt",
		"git push origin feature/x":            "git push origin feature/x",
		"head -n 5 a":                          "head -n 5 a",
		"cp ./a.txt ~/b /etc/hosts":            "cp <PATH> <PATH> <PATH>",
		"kubectl -n prod get pods":             "kubectl -n <NS> get pods",
		"ffmpeg -i in.mp4 out.webm":            "ffmpeg -i <PATH> out.webm",
		"--version":                            "--version",
	}
	for in, want := range cases {
		if got := normalizeCommand(in); got != want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", in, got, want)
		}
		if got := normalizeCommand(want); got != want {
			t.Errorf("normalizeCommand not idempotent on %q: %q", want, got)
		}
	}
}

func TestIsIgnorable(t *testing.T) {
	for s, want := range map[string]bool{
		"ls": true, "ls -la": true, "cd /tmp": true, "# note": true, "  ": true,
		"for f in *; do rm -rf $f; done": true, "lsof -i :8080": false, "lsblk": false, "cdk deploy": false,
	} {
		if got := isIgnorable(s); got != want {
			t.Errorf("isIgnorable(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestScrub(t *testing.T) {
	cases := map[string]string{
		"mysql -u root -pHunter2 db":                 "mysql -u root -p*** db",
		"psql --password hunter2":                    "psql --password ***",
		"curl -u alice:hunter2 example.com":          "curl -u *** example.com",
		"git clone https://bob:hunter2@github.com/x": "git clone https://***@github.com/x",
		"echo ghp_abcdefghijklmno":                   "echo <SECRET>",
		"export GITHUB_TOKEN=abc":                    "export GITHUB_TOKEN=***",
		"curl -H Authorization: Bearer abc.def":      "curl -H Authorization: Bearer ***",
		"find . -print":                              "find . -print",
	}
	for in, want := range cases {
		if got := scrub(in); got != want {
			t.Errorf("scrub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadHistoryFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".zsh_history")
	data := ": 1700000000:0;for f in *; do\\\n  echo $f\\\ndone\n: 1700000001:0;echo caf\xc3\x83\xa9\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readHistoryFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %q", got)
	}
	if got[1] != ": 1700000001:0;echo cafÉ" {
		t.Errorf("unmetafy: got %q", got[1])
	}
}

func TestGenerateCardsUpdatesHistoryCount(t *testing.T) {
	cmd := "git rebase -i --autosquash origin"
	existing := []Card{{ID: hash(cmd), HistoryCount: 1}}
	out := GenerateCards([]CommandEvent{{Command: cmd, Count: 4}}, existing)
	if len(out) != 0 || existing[0].HistoryCount != 4 {
		t.Errorf("new=%d count=%d", len(out), existing[0].HistoryCount)
	}
}

func TestClozeSkipsSudoAndRejectsCommandName(t *testing.T) {
	_, answer, _ := cloze("sudo systemctl restart --now nginx")
	if answer != "--now" {
		t.Errorf("answer = %q, want --now", answer)
	}
	if _, answer, _ := cloze("make <PATH>"); answer != "" {
		t.Errorf("answer = %q, want none", answer)
	}
	if tags := deriveTags("sudo pacman -Syu --needed"); tags[0] != "pacman" {
		t.Errorf("tags = %v", tags)
	}
}

func TestGenerateCardsOnePerToolAnswer(t *testing.T) {
	events := []CommandEvent{
		{Command: "rm -rf build dist", Count: 1},
		{Command: "rm -rf node_modules out", Count: 9},
		{Command: "git log --oneline --graph", Count: 2},
		{Command: "echo hi | wc", Count: 1},
	}
	out := GenerateCards(events, nil)
	if len(out) != 2 {
		t.Fatalf("want 2 cards, got %d: %+v", len(out), out)
	}
	if out[0].Command != "rm -rf node_modules out" {
		t.Errorf("most-used variant should win, got %q", out[0].Command)
	}
	if again := GenerateCards([]CommandEvent{{Command: "rm -rf tmp x", Count: 3}}, out); len(again) != 0 {
		t.Errorf("existing tool+answer should block new card, got %+v", again)
	}
}

func TestIsTrickySingleLongFlag(t *testing.T) {
	if isTricky("git push --tags") {
		t.Error("one long flag should not be tricky")
	}
}
