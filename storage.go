package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Card represents a single flashcard generated from a shell command.
type Card struct {
	ID           string    `json:"id"` // stable hash of normalized command
	Prompt       string    `json:"prompt"`
	Answer       string    `json:"answer"` // often the hidden flag or full command
	Hint         string    `json:"hint"`
	Command      string    `json:"command"` // original (scrubbed)
	Tags         []string  `json:"tags"`
	Box          int       `json:"box"` // 1..5 (Leitner)
	NextDue      time.Time `json:"next_due"`
	LastReviewed time.Time `json:"last_reviewed"`
	Streak       int       `json:"streak"`
	TimesSeen    int       `json:"times_seen"`
	SeenCount    int       `json:"seen_count"`
}

// Load/Save to JSON in XDG data dir.
func dataDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "memento"), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".local", "share", "memento"), nil
}

func cardsPath() (string, error) {
	d, err := dataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(d, "cards.json"), nil
}

func LoadCards() ([]Card, error) {
	p, err := cardsPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return []Card{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cards []Card
	if err := json.Unmarshal(b, &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

func SaveCards(cards []Card) error {
	p, err := cardsPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cards, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func UpsertCards(existing []Card, incoming []Card) []Card {
	idx := map[string]int{}
	for i, c := range existing {
		idx[c.ID] = i
	}
	for _, c := range incoming {
		if i, ok := idx[c.ID]; ok {
			// merge lightweight updates (e.g., tags)
			existing[i].Tags = union(existing[i].Tags, c.Tags)
			if existing[i].Prompt == "" {
				existing[i].Prompt = c.Prompt
			}
			if existing[i].Answer == "" {
				existing[i].Answer = c.Answer
			}
			if existing[i].Hint == "" {
				existing[i].Hint = c.Hint
			}
		} else {
			existing = append(existing, c)
		}
	}
	return existing
}

func union(a, b []string) []string {
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (c *Card) Due(now time.Time) bool { return !now.Before(c.NextDue) }

func (c *Card) Touch(now time.Time) { c.LastReviewed = now; c.TimesSeen++ }

func (c *Card) String() string { return fmt.Sprintf("[%d] %s", c.Box, c.Prompt) }
