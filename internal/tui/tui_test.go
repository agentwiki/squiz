package tui

import (
	"github.com/agentwiki/squiz/internal/store"
	"github.com/charmbracelet/lipgloss"
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

func TestSameQIDRefreshPreservesDraft(t *testing.T) {
	m := choiceModel(t)
	m.custom = true
	m.input.SetValue("작성 중")
	sc := *m.screen
	sc.Notice = "바뀐 조건"
	if err := m.x.PublishQuestion(sc); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tickMsg{})
	m = next.(model)
	if m.screen.Notice != "바뀐 조건" || m.input.Value() != "작성 중" || !m.custom {
		t.Fatalf("refresh lost draft: %#v", m.screen)
	}
}

func TestChoiceAlternativeInput(t *testing.T) {
	m := choiceModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("팀이 함께 사용")})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(model)
	a, err := m.x.PeekResponse()
	if err != nil || a == nil || a.Action != engine.AnswerProposal || a.Text != "팀이 함께 사용" {
		t.Fatalf("%#v %v", a, err)
	}
}

func TestKoreanWidthAndResize(t *testing.T) {
	m := choiceModel(t)
	m.screen.Question.Text = strings.Repeat("한국어 조건과 경계를 확인하세요. ", 8)
	for _, w := range []int{55, 156, 30, 55} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 100})
		m = next.(model)
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > w {
				t.Fatalf("width %d exceeded: %q", w, line)
			}
		}
		if !strings.Contains(strings.Join(strings.Fields(m.View()), " "), "현재 개념 전체") {
			t.Fatal("skip scope not visible", m.View())
		}
	}
}

func TestStartupAndSessionSwitch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(nil)
	m.store = st
	next, _ := m.Update(tickMsg{})
	m = next.(model)
	if m.x != nil || !strings.Contains(m.View(), "초기화") {
		t.Fatal("must wait before init")
	}
	s, err := st.Create(engine.SourceConcept, engine.RoleNone)
	if err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(tickMsg{})
	m = next.(model)
	if m.x.Dir != st.QueueDir(s.ID) {
		t.Fatal("not connected")
	}
	s2, _ := st.Create(engine.SourceConcept, engine.RoleNone)
	next, _ = m.Update(tickMsg{})
	m = next.(model)
	if m.x.Dir != st.QueueDir(s2.ID) {
		t.Fatal("not following active session")
	}
	if err := m.x.PublishStatus(ipc.Status{Seq: s2.Seq, State: "done", Text: "세션이 완료되었습니다."}); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(tickMsg{})
	m = next.(model)
	if !strings.Contains(m.View(), "완료") || strings.Contains(m.View(), "질문을 기다리는") {
		t.Fatal(m.View())
	}
}

func TestTerminalStatusWinsOverStaleQuestion(t *testing.T) {
	m := choiceModel(t)
	if err := m.x.PublishQuestion(*m.screen); err != nil {
		t.Fatal(err)
	}
	if err := m.x.PublishStatus(ipc.Status{State: "aborted", Text: "세션을 중단했습니다."}); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tickMsg{})
	m = next.(model)
	if m.screen != nil || !strings.Contains(m.View(), "중단") {
		t.Fatal("stale question hid terminal state")
	}
}

func TestEventReplayHealsMissingTerminalPublication(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.Create(engine.SourceConcept, engine.RoleNone)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []struct {
		typ string
		p   any
	}{
		{engine.EvConceptAdd, engine.ConceptAddPayload{ConceptParams: engine.ConceptParams{Name: "범위", ClaimText: "가설", ClaimType: engine.ClaimFact}}},
		{engine.EvVerify, engine.VerifyParams{ConceptID: "c1", Status: engine.VerifyContested}},
		{engine.EvStart, struct{}{}}, {engine.EvAsk, engine.AskParams{Kind: engine.KindOpen, Text: "질문"}},
	} {
		if err := st.Commit(s, ev.typ, ev.p); err != nil {
			t.Fatal(err)
		}
	}
	x, _ := ipc.New(st.QueueDir(s.ID))
	if err := x.PublishQuestion(ipc.Screen{Question: s.Pending}); err != nil {
		t.Fatal(err)
	}
	if err := x.PublishStatus(ipc.StatusFor(s)); err != nil {
		t.Fatal(err)
	}
	m := newModel(nil)
	m.store = st
	next, _ := m.Update(tickMsg{})
	m = next.(model)
	if err := st.Commit(s, engine.EvAnswer, engine.Answer{QID: s.Pending.QID, Action: engine.AnswerAbort}); err != nil {
		t.Fatal(err)
	}
	// Simulate process death after commit: no ClearQuestion or PublishStatus.
	next, _ = m.Update(tickMsg{})
	m = next.(model)
	if m.screen != nil || m.progress.State != "aborted" {
		t.Fatalf("%#v", m.progress)
	}
	// A new UI must also favor replay over stale transport files.
	fresh := newModel(nil)
	fresh.store = st
	next, _ = fresh.Update(tickMsg{})
	fresh = next.(model)
	if fresh.screen != nil || fresh.progress.State != "aborted" {
		t.Fatalf("reconnect: %#v", fresh.progress)
	}
}
