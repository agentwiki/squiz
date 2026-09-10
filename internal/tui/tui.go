// Package tui is the learner-side terminal UI (`squiz ui`). One screen, one
// question (design §4.6); the header always shows concept · stage (A7).
// Slash commands map to the autonomy exits: /clarify /explain /skip /object
// /abort (squiz-core.md §3).
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agentwiki/squiz/internal/engine"
	"github.com/agentwiki/squiz/internal/ipc"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Padding(0, 1)
	noticeStyle   = lipgloss.NewStyle().Faint(true).Italic(true)
	questionStyle = lipgloss.NewStyle().Padding(1, 2).Width(76)
	optionStyle   = lipgloss.NewStyle().PaddingLeft(4)
	helpStyle     = lipgloss.NewStyle().Faint(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	waitStyle     = lipgloss.NewStyle().Faint(true).Padding(2, 2)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

var confidences = []engine.Confidence{engine.Sure, engine.Unsure, engine.DontKnow}
var confLabels = map[engine.Confidence]string{
	engine.Sure: "확신", engine.Unsure: "불확실", engine.DontKnow: "모르겠음",
}

type tickMsg time.Time

type model struct {
	x             *ipc.Exchange
	screen        *ipc.Screen
	answered      string // qid already answered, waiting for the next screen
	answeredTicks int    // ticks the answered qid kept reappearing
	input         textarea.Model
	confIdx       int
	optionIdx     int // keyboard/mouse focus
	choice        int // selected option, one-based; selection is not submission
	status        string
	errText       string
	width         int
}

func newModel(x *ipc.Exchange) model {
	ta := textarea.New()
	ta.Placeholder = "답을 입력하세요…"
	ta.SetHeight(5)
	ta.SetWidth(76)
	ta.Focus()
	return model{x: x, input: ta, confIdx: 1} // default: unsure — honest default
}

func (m model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := msg.Width - 4
		if w > 100 {
			w = 100
		}
		if w > 10 {
			m.input.SetWidth(w)
		}
		return m, nil
	case tickMsg:
		sc, err := m.x.ReadQuestion()
		if err != nil {
			m.errText = err.Error()
			return m, tick()
		}
		if sc == nil || sc.Question == nil {
			m.screen = nil
			return m, tick()
		}
		if m.screen == nil || m.screen.Question.QID != sc.Question.QID {
			show, reshow := sc.Question.QID != m.answered, false
			if !show {
				// the answered question is still posted: normally the CLI
				// clears it right after approval. If it lingers (~3s), the
				// response was rejected or lost — re-show so the user can
				// answer again instead of deadlocking.
				m.answeredTicks++
				if m.answeredTicks > 8 {
					show, reshow = true, true
				}
			}
			if show {
				m.screen = sc
				m.answered = ""
				m.answeredTicks = 0
				m.input.Reset()
				m.optionIdx = 0
				m.choice = 0
				m.errText = ""
				if reshow {
					m.status = "응답이 접수되지 않아 같은 질문을 다시 표시합니다."
				} else {
					m.status = ""
				}
			}
		}
		return m, tick()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.hasOptions() {
				m.optionIdx = (m.optionIdx + 1) % (len(m.screen.Question.Options) + len(actionButtons))
				return m, nil
			} else if m.screen != nil {
				m.confIdx = (m.confIdx + 1) % len(confidences)
				return m, nil
			}
		case "up", "k":
			if m.hasOptions() {
				total := len(m.screen.Question.Options) + len(actionButtons)
				m.optionIdx = (m.optionIdx - 1 + total) % total
				return m, nil
			}
		case "down", "j":
			if m.hasOptions() {
				m.optionIdx = (m.optionIdx + 1) % (len(m.screen.Question.Options) + len(actionButtons))
				return m, nil
			}
		case "enter":
			if m.hasOptions() {
				return m.activateFocused()
			}
		case "ctrl+d", "ctrl+s":
			if m.screen != nil {
				return m.submit()
			}
		}
		if m.hasOptions() {
			if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(m.screen.Question.Options) {
				m.choice, m.optionIdx = n, n-1
				m.status, m.errText = fmt.Sprintf("%d번을 선택했습니다. '답변 제출'을 눌러 확정하세요.", n), ""
				return m, nil
			}
			// Choice screens are deliberately modal: letter keys navigate and
			// cannot accidentally leave invisible text in the textarea.
			return m, nil
		}
	case tea.MouseMsg:
		if m.screen == nil {
			return m, nil
		}
		mouse := tea.MouseEvent(msg)
		if m.hasOptions() {
			total := len(m.screen.Question.Options) + len(actionButtons)
			switch mouse.Button {
			case tea.MouseButtonWheelUp:
				m.optionIdx = (m.optionIdx - 1 + total) % total
				return m, nil
			case tea.MouseButtonWheelDown:
				m.optionIdx = (m.optionIdx + 1) % total
				return m, nil
			case tea.MouseButtonLeft:
				if mouse.Action == tea.MouseActionPress {
					if choice, ok := m.choiceAtRow(mouse.Y); ok {
						m.optionIdx = choice
						m.choice = choice + 1
						m.status, m.errText = fmt.Sprintf("%d번을 선택했습니다. '답변 제출'을 눌러 확정하세요.", m.choice), ""
						return m, nil
					}
					if button, ok := m.buttonAtRow(mouse.Y); ok {
						m.optionIdx = len(m.screen.Question.Options) + button
						return m.activateButton(button)
					}
				}
			}
		} else if mouse.Button == tea.MouseButtonLeft && mouse.Action == tea.MouseActionPress {
			if confidence, ok := m.confidenceAtRow(mouse.Y); ok {
				m.confIdx = confidence
				return m, nil
			}
			if button, ok := m.buttonAtRow(mouse.Y); ok {
				return m.activateButton(button)
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// choiceAtRow translates Bubble Tea's zero-based mouse row to the option
// rendered on that row. Computing it from the rendered prefix keeps hit
// targets correct when the question wraps or a notice is present.
func (m model) choiceAtRow(y int) (int, bool) {
	start := lipgloss.Height(m.questionPrefix())
	i := y - start
	return i, i >= 0 && i < len(m.screen.Question.Options)
}

func (m model) confidenceAtRow(y int) (int, bool) {
	start := lipgloss.Height(m.questionPrefix()) + lipgloss.Height(m.input.View())
	i := y - start
	return i, i >= 0 && i < len(confidences)
}

type actionButton struct {
	label  string
	action engine.AnswerAction
}

var actionButtons = []actionButton{
	{label: "답변 제출"},
	{label: "질문 명확화", action: engine.AnswerClarify},
	{label: "설명 요청", action: engine.AnswerExplain},
	{label: "건너뛰기", action: engine.AnswerSkip},
	{label: "이의 제기", action: engine.AnswerObject},
	{label: "세션 중단", action: engine.AnswerAbort},
}

func (m model) buttonStartRow() int {
	start := lipgloss.Height(m.questionPrefix())
	if m.hasOptions() {
		return start + len(m.screen.Question.Options) + 1
	}
	return start + lipgloss.Height(m.input.View()) + len(confidences) + 1
}

func (m model) buttonAtRow(y int) (int, bool) {
	i := y - m.buttonStartRow()
	return i, i >= 0 && i < len(actionButtons)
}

func (m model) activateFocused() (tea.Model, tea.Cmd) {
	if m.optionIdx < len(m.screen.Question.Options) {
		m.choice = m.optionIdx + 1
		m.status, m.errText = fmt.Sprintf("%d번을 선택했습니다. '답변 제출'을 눌러 확정하세요.", m.choice), ""
		return m, nil
	}
	return m.activateButton(m.optionIdx - len(m.screen.Question.Options))
}

func (m model) activateButton(i int) (tea.Model, tea.Cmd) {
	if i == 0 {
		if m.hasOptions() {
			if m.choice == 0 {
				m.errText = "먼저 선택지를 고르세요."
				return m, nil
			}
			return m.submitChoice(m.choice)
		}
		return m.submit()
	}
	return m.publish(engine.Answer{QID: m.screen.Question.QID, Action: actionButtons[i].action})
}

func (m model) questionPrefix() string {
	if m.screen == nil || m.screen.Question == nil {
		return ""
	}
	q := m.screen.Question
	head := "squiz"
	if m.screen.ConceptName != "" {
		head = m.screen.ConceptName
		if m.screen.Stage != "" {
			head += " · " + stageLabel(m.screen.Stage)
		}
	} else if q.Kind != "" {
		head += " · " + string(q.Kind)
	}
	counts := fmt.Sprintf("  판정 %d · 화면 %d", m.screen.JudgedCount, m.screen.BurdenCount)
	prefix := headerStyle.Render(head) + helpStyle.Render(counts) + "\n"
	if m.screen.Notice != "" {
		prefix += noticeStyle.Render("· "+m.screen.Notice) + "\n"
	}
	return prefix + questionStyle.Render(q.Text) + "\n"
}

func (m model) hasOptions() bool {
	return m.screen != nil && m.screen.Question != nil && len(m.screen.Question.Options) > 0
}

func (m model) submitChoice(choice int) (tea.Model, tea.Cmd) {
	q := m.screen.Question
	a := engine.Answer{QID: q.QID, Action: engine.AnswerChoice, Choice: choice}
	return m.publish(a)
}

func (m model) submit() (tea.Model, tea.Cmd) {
	q := m.screen.Question
	raw := strings.TrimSpace(m.input.Value())
	a := engine.Answer{QID: q.QID}

	if strings.HasPrefix(raw, "/") {
		parts := strings.SplitN(raw, " ", 2)
		reason := ""
		if len(parts) > 1 {
			reason = parts[1]
		}
		switch parts[0] {
		case "/clarify":
			a.Action = engine.AnswerClarify
		case "/explain":
			a.Action, a.Reason = engine.AnswerExplain, reason
		case "/skip":
			a.Action = engine.AnswerSkip
		case "/object":
			a.Action, a.Reason = engine.AnswerObject, reason
		case "/abort":
			a.Action = engine.AnswerAbort
		default:
			m.errText = "알 수 없는 명령: " + parts[0]
			return m, nil
		}
	} else if len(q.Options) > 0 {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > len(q.Options) {
			m.errText = fmt.Sprintf("1~%d 중 번호를 입력하세요", len(q.Options))
			return m, nil
		}
		a.Action = engine.AnswerChoice
		a.Choice = n
	} else {
		if raw == "" {
			m.errText = "빈 답은 보낼 수 없습니다. 모르면 확신도를 '모르겠음'으로 두고 그렇게 적어주세요 — 불이익은 없습니다."
			return m, nil
		}
		a.Action = engine.AnswerText
		a.Text = raw
		a.Confidence = confidences[m.confIdx]
	}
	return m.publish(a)
}

func (m model) publish(a engine.Answer) (tea.Model, tea.Cmd) {
	if err := m.x.PublishResponse(a); err != nil {
		m.errText = err.Error()
		return m, nil
	}
	m.answered = a.QID
	m.screen = nil
	m.input.Reset()
	m.status = "답변을 보냈습니다. 다음 질문을 기다리는 중…"
	m.errText = ""
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	if m.screen == nil {
		b.WriteString(waitStyle.Render("··· 질문을 기다리는 중 (AI가 준비되면 여기 나타납니다)"))
		if m.status != "" {
			b.WriteString("\n" + waitStyle.Render(m.status))
		}
		if m.errText != "" {
			b.WriteString("\n" + errStyle.Render(m.errText))
		}
		b.WriteString("\n\n" + helpStyle.Render("Ctrl+C 종료"))
		return b.String()
	}
	q := m.screen.Question

	// A7: concept · stage always visible in the header.
	b.WriteString(m.questionPrefix())
	if len(q.Options) > 0 {
		for i, opt := range q.Options {
			line := fmt.Sprintf("○ %d  %s", i+1, opt)
			if i+1 == m.choice {
				line = selectedStyle.Render(fmt.Sprintf("● %d  %s", i+1, opt))
			}
			if i == m.optionIdx {
				line = selectedStyle.Render("› " + line)
			}
			b.WriteString(optionStyle.Render(line) + "\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString(m.input.View() + "\n")
		for i, confidence := range confidences {
			mark := "○"
			if i == m.confIdx {
				mark = "●"
			}
			b.WriteString(optionStyle.Render(fmt.Sprintf("%s 확신도: %s", mark, confLabels[confidence])) + "\n")
		}
		b.WriteString("\n")
	}
	for i, button := range actionButtons {
		line := "[ " + button.label + " ]"
		if m.hasOptions() && m.optionIdx == len(q.Options)+i {
			line = selectedStyle.Render("› " + line)
		}
		b.WriteString(optionStyle.Render(line) + "\n")
	}
	if m.status != "" {
		b.WriteString(noticeStyle.Render(m.status) + "\n")
	}
	if m.errText != "" {
		b.WriteString(errStyle.Render(m.errText) + "\n")
	}
	b.WriteString(helpStyle.Render("클릭 또는 ↑/↓·j/k로 이동 · Enter로 선택/버튼 실행 · Ctrl+C TUI 종료"))
	return b.String()
}

func stageLabel(st engine.Stage) string {
	switch st {
	case engine.StagePredict:
		return "예측"
	case engine.StageWhy:
		return "근거"
	case engine.StageBoundary:
		return "경계"
	case engine.StageTransfer:
		return "전이"
	}
	return string(st)
}

// Run starts the TUI against a session queue directory.
func Run(queueDir string) (int, error) {
	x, err := ipc.New(queueDir)
	if err != nil {
		return 1, err
	}
	p := tea.NewProgram(newModel(x), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return 1, err
	}
	return 0, nil
}
