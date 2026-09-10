package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agentwiki/squiz/internal/engine"
	"github.com/agentwiki/squiz/internal/ipc"
)

func choiceModel(t *testing.T) model {
	t.Helper()
	x, err := ipc.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return model{
		x: x,
		screen: &ipc.Screen{Question: &engine.Question{
			QID: "q1", Text: "가장 중요한 사용자는?",
			Options: []string{"개인 사용자", "운영자", "아직 모르겠음"},
		}},
		input: newModel(x).input,
	}
}

func TestChoiceScreenNavigatesAndSubmitsWithEnter(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.optionIdx != 1 {
		t.Fatalf("option index = %d, want 1", m.optionIdx)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a == nil || a.Action != engine.AnswerChoice || a.Choice != 2 {
		t.Fatalf("response = %#v, want choice 2", a)
	}
	if m.screen != nil {
		t.Fatal("screen should switch to waiting after submission")
	}
}

func TestChoiceScreenDigitSubmitsImmediately(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a == nil || a.Choice != 3 {
		t.Fatalf("response = %#v, want choice 3", a)
	}
}

func TestChoiceScreenKeepsAutonomyShortcuts(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a == nil || a.Action != engine.AnswerClarify {
		t.Fatalf("response = %#v, want clarify", a)
	}
}

func TestChoiceViewIsModal(t *testing.T) {
	m := choiceModel(t)
	view := m.View()
	if !strings.Contains(view, "Enter 선택") || strings.Contains(view, "답을 입력하세요") {
		t.Fatalf("choice view should show navigation help without textarea:\n%s", view)
	}
}
