package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func TestChoiceScreenSelectsThenSubmitsWithButton(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.optionIdx != 1 {
		t.Fatalf("option index = %d, want 1", m.optionIdx)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.choice != 2 {
		t.Fatalf("choice = %d, want selected choice 2", m.choice)
	}
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a != nil {
		t.Fatalf("selecting an option must not submit it: %#v", a)
	}

	// Move from option 2 through option 3 to the submit button.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	a, err = m.x.PeekResponse()
	if err != nil || a == nil || a.Choice != 2 {
		t.Fatalf("submit response = %#v, err = %v", a, err)
	}
}

func TestChoiceScreenDigitOnlySelects(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(model)
	if m.choice != 3 {
		t.Fatalf("choice = %d, want 3", m.choice)
	}
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a != nil {
		t.Fatalf("digit must not submit: %#v", a)
	}
}

func TestChoiceViewIsModal(t *testing.T) {
	m := choiceModel(t)
	view := m.View()
	if !strings.Contains(view, "답변 제출") || !strings.Contains(view, "설명 요청") || strings.Contains(view, "답을 입력하세요") {
		t.Fatalf("choice view should show navigation help without textarea:\n%s", view)
	}
}

func TestChoiceScreenClickSelectsThenSubmitButtonPublishes(t *testing.T) {
	m := choiceModel(t)
	row := lipgloss.Height(m.questionPrefix()) + 1
	next, _ := m.Update(tea.MouseMsg{X: 8, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil {
		t.Fatal(err)
	}
	if a != nil || m.choice != 2 {
		t.Fatalf("click should select without submitting: choice=%d response=%#v", m.choice, a)
	}
	next, _ = m.Update(tea.MouseMsg{X: 8, Y: m.buttonStartRow(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	a, err = m.x.PeekResponse()
	if err != nil || a == nil || a.Choice != 2 {
		t.Fatalf("submit response = %#v, err = %v", a, err)
	}
}

func TestActionButtonPublishesWithoutSlashCommand(t *testing.T) {
	m := choiceModel(t)
	row := m.buttonStartRow() + 2 // 설명 요청
	next, _ := m.Update(tea.MouseMsg{X: 8, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil || a == nil || a.Action != engine.AnswerExplain {
		t.Fatalf("response = %#v, err = %v", a, err)
	}
}

func TestFreeTextConfidenceCanBeClicked(t *testing.T) {
	m := choiceModel(t)
	m.screen.Question.Options = nil
	row := lipgloss.Height(m.questionPrefix()) + lipgloss.Height(m.input.View()) + 2
	next, _ := m.Update(tea.MouseMsg{X: 8, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(model)
	if m.confIdx != 2 {
		t.Fatalf("confidence index = %d, want dont-know index 2", m.confIdx)
	}
}

func TestChoiceScreenMouseWheelMovesSelection(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m = next.(model)
	if m.optionIdx != 1 {
		t.Fatalf("option index = %d, want 1", m.optionIdx)
	}
}
