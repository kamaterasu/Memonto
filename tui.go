package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"time"
)

type model struct {
	cards    []Card
	idx      int
	input    textinput.Model
	progress progress.Model
	feedback string
	checking bool
	quit     bool
}

func initialModel(cards []Card) model {
	m := model{cards: DueCards(cards, time.Now())}
	if len(m.cards) == 0 {
		return m
	}
	m.input = textinput.New()
	m.input.Placeholder = "your answer (flag/word)"
	m.input.Focus()
	m.progress = progress.New(progress.WithDefaultGradient())
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) View() string {
	st := lipgloss.NewStyle().Margin(1, 2)
	if len(m.cards) == 0 {
		return st.Render("Nothing due. You're done for today. ✨")
	}
	c := m.cards[m.idx]
	header := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("[%d/%d] Tags: %s", m.idx+1, len(m.cards), strings.Join(c.Tags, ", ")))
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(c.Prompt)
	bar := m.progress.ViewAs(float64(m.idx) / float64(len(m.cards)))
	fb := m.feedback
	hint := "(enter=check)"
	if m.checking {
		hint = "(n=next, q=quit)"
	}
	return st.Render(header + "\n\n" + prompt + "\n\n" + m.input.View() + "\n\n" + bar + "\n\n" + fb + "\n" + hint)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quit = true
			return m, tea.Quit
		case "enter":
			if len(m.cards) == 0 {
				return m, tea.Quit
			}
			ans := strings.TrimSpace(m.input.Value())
			correct := checkAnswer(m.cards[m.idx], ans)
			Grade(&m.cards[m.idx], correct, time.Now())
			m.feedback = feedbackLine(correct, m.cards[m.idx])
			_ = SaveProgress(m.cards[m.idx])
			m.checking = true
			m.input.Blur()
			return m, nil
		case "n", "right", "tab":
			if !m.checking {
				break
			}
			if m.idx < len(m.cards)-1 {
				m.idx++
				m.feedback = ""
				m.checking = false
				m.input.SetValue("")
				m.input.Focus()
			} else {
				return m, tea.Quit
			}
		case "q":
			if !m.checking {
				break
			}
			m.quit = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func checkAnswer(c Card, ans string) bool {
	// Basic: exact match or contained; case-insensitive.
	if ans == "" {
		return false
	}
	A := strings.ToLower(strings.TrimSpace(c.Answer))
	B := strings.ToLower(strings.TrimSpace(ans))
	return A == B || strings.Contains(A, B) || strings.Contains(B, A)
}

func feedbackLine(ok bool, c Card) string {
	if ok {
		return "✔ Correct → " + c.Answer
	}
	return "✘ Nope. Correct: " + c.Answer + hintStr(c.Hint)
}

func hintStr(h string) string {
	if h == "" {
		return ""
	}
	return "\t( hint: " + h + " )"
}

func RunTUI(all []Card) error {
	p := tea.NewProgram(initialModel(all))
	_, err := p.Run()
	return err
}

// Persist only the updated card; keep it simple by reloading and merging.
func SaveProgress(updated Card) error {
	cards, err := LoadCards()
	if err != nil {
		return err
	}
	for i := range cards {
		if cards[i].ID == updated.ID {
			cards[i] = updated
			return SaveCards(cards)
		}
	}
	cards = append(cards, updated)
	return SaveCards(cards)
}
