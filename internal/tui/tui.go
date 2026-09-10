// Package tui is the learner-side terminal UI (`squiz ui`). One screen, one
// question (design §4.6); the header always shows concept · stage (A7).
// Slash commands map to the autonomy exits: /clarify /explain /skip /object
// /abort (squiz-core.md §3).
package tui

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agentwiki/squiz/internal/store"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	confStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
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
	store         *store.Store
	sessionID     string
	progress      *ipc.Status
	custom        bool
	height        int
	scroll        int
	x             *ipc.Exchange
	screen        *ipc.Screen
	answered      string // qid already answered, waiting for the next screen
	answeredTicks int    // ticks the answered qid kept reappearing
	input         textarea.Model
	confIdx       int
	optionIdx     int
	status        string
	errText       string
	width         int
}

func newModel(x *ipc.Exchange) model {
	ta := textarea.New()
	ta.Placeholder = "답을 입력하세요… (/clarify /explain /skip /object /abort)"
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
		m.height = msg.Height
		w := msg.Width - 4
		if w > 100 {
			w = 100
		}
		if w > 10 {
			m.input.SetWidth(w)
		}
		return m, nil
	case tickMsg:
		if m.store != nil {
			id, err := m.store.ActiveID()
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					m.errText = err.Error()
				}
				return m, tick()
			}
			if m.x == nil || m.x.Dir != m.store.QueueDir(id) {
				x, err := ipc.New(m.store.QueueDir(id))
				if err != nil {
					m.errText = err.Error()
					return m, tick()
				}
				m.x = x
				m.sessionID = id
				m.screen = nil
				m.answered = ""
				m.status = ""
				m.errText = ""
				m.input.Reset()
				m.confIdx = 1
				if session, err := m.store.Load(id); err == nil {
					st := ipc.StatusFor(session)
					m.progress = &st
				}
			}
		}
		// The event log also heals a crash between CLI commit and status publication.
		if m.store != nil {
			if session, err := m.store.Load(m.sessionID); err != nil {
				m.errText = err.Error()
			} else if m.progress == nil || session.Seq > m.progress.Seq {
				st := ipc.StatusFor(session)
				m.progress = &st
			}
		}
		if m.x == nil {
			return m, tick()
		}
		if st, err := m.x.ReadStatus(); err != nil {
			m.errText = err.Error()
		} else if st != nil && (m.progress == nil || st.Seq >= m.progress.Seq) {
			m.progress = st
		}
		if m.progress != nil && (m.progress.State == "done" || m.progress.State == "aborted") {
			m.screen = nil
			return m, tick()
		}
		sc, err := m.x.ReadQuestion()
		if err != nil {
			m.errText = err.Error()
			return m, tick()
		}
		if sc == nil || sc.Question == nil {
			m.screen = nil
			return m, tick()
		}
		if m.screen != nil && m.screen.Question.QID == sc.Question.QID {
			m.screen = sc
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
				m.custom = false
				m.scroll = 0
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
		case "pgup":
			m.scroll += max(1, m.height/2)
			return m, nil
		case "pgdown":
			m.scroll = max(0, m.scroll-max(1, m.height/2))
			return m, nil
		case "esc":
			if m.custom {
				m.custom = false
				m.input.Reset()
				return m, nil
			}
		case "i":
			if m.hasOptions() && m.screen.Question.Kind != engine.KindSessionLimit {
				m.custom = true
				return m, nil
			}
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.screen != nil {
				m.confIdx = (m.confIdx + 1) % len(confidences)
				return m, nil
			}
		case "up", "k":
			if m.hasOptions() {
				m.optionIdx = (m.optionIdx - 1 + len(m.screen.Question.Options)) % len(m.screen.Question.Options)
				return m, nil
			}
		case "down", "j":
			if m.hasOptions() {
				m.optionIdx = (m.optionIdx + 1) % len(m.screen.Question.Options)
				return m, nil
			}
		case "enter":
			if m.hasOptions() {
				return m.submitChoice(m.optionIdx + 1)
			}
		case "ctrl+d", "ctrl+s":
			if m.screen != nil {
				return m.submit()
			}
		}
		if m.hasOptions() {
			if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(m.screen.Question.Options) {
				return m.submitChoice(n)
			}
			actions := map[string]engine.AnswerAction{
				"c": engine.AnswerClarify, "e": engine.AnswerExplain,
				"s": engine.AnswerSkip, "o": engine.AnswerObject, "a": engine.AnswerAbort,
			}
			if action, ok := actions[msg.String()]; ok {
				return m.publish(engine.Answer{QID: m.screen.Question.QID, Action: action})
			}
			// Choice screens are deliberately modal: letter keys navigate and
			// cannot accidentally leave invisible text in the textarea.
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) hasOptions() bool {
	return !m.custom && m.screen != nil && m.screen.Question != nil && len(m.screen.Question.Options) > 0
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
	} else if len(q.Options) > 0 && !m.custom {
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
		if m.custom {
			a.Action = engine.AnswerProposal
		}
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
	w := m.width
	if w <= 0 {
		w = 80
	}
	content := ansi.Hardwrap(ansi.Wrap(m.content(), w, ""), w, true)
	if m.height > 2 {
		lines := strings.Split(content, "\n")
		if len(lines) > m.height {
			end := max(m.height-1, len(lines)-m.scroll)
			end = min(end, len(lines))
			return strings.Join(lines[max(0, end-m.height+1):end], "\n") + "\n" + ansi.Wrap("PgUp/PgDn 이전 내용·아래 보기", w, "")
		}
	}
	return content
}
func (m model) content() string {
	var b strings.Builder
	if m.screen == nil {
		if m.progress != nil {
			b.WriteString(m.progress.Text + "\n")
			for _, text := range m.progress.Messages {
				b.WriteString(text + "\n")
			}
		} else {
			b.WriteString("세션 초기화를 기다립니다. 같은 디렉터리에서 AI가 squiz init을 실행하면 자동 연결됩니다.")
		}
		if m.status != "" && (m.progress == nil || m.progress.State == "question") {
			b.WriteString("\n" + waitStyle.Render(m.status))
		}
		if m.errText != "" {
			b.WriteString("\n" + errStyle.Render(m.errText))
		}
		b.WriteString("\n\n" + helpStyle.Render("Ctrl+C 종료"))
		return b.String()
	}
	q := m.screen.Question

	// A7: concept · stage always visible in the header
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
	b.WriteString(headerStyle.Render(head) + helpStyle.Render(counts) + "\n")

	if m.progress != nil {
		for _, text := range m.progress.Messages {
			b.WriteString(noticeStyle.Render(text) + "\n")
		}
		if m.progress.State == "error" {
			b.WriteString(errStyle.Render(m.progress.Text) + "\n")
		}
	}
	if m.screen.Notice != "" {
		b.WriteString(noticeStyle.Render("· "+m.screen.Notice) + "\n")
	}
	b.WriteString(questionStyle.Width(m.questionWidth()).Render(q.Text) + "\n")
	if m.hasOptions() {
		for i, opt := range q.Options {
			line := fmt.Sprintf("  %d) %s", i+1, opt)
			if i == m.optionIdx {
				line = selectedStyle.Render("› " + line[2:])
			}
			b.WriteString(optionStyle.Render(line) + "\n")
		}
		b.WriteString("\n" + helpStyle.Render("↑/↓ 또는 j/k로 이동 · Enter 선택 · 숫자 즉시 선택") + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	if !m.hasOptions() {
		b.WriteString("확신도: " + confStyle.Render(confLabels[confidences[m.confIdx]]) +
			helpStyle.Render("  (Tab으로 변경 — '모르겠음'도 정상 경로입니다)") + "\n")
	}
	if m.status != "" {
		b.WriteString(noticeStyle.Render(m.status) + "\n")
	}
	if m.errText != "" {
		b.WriteString(errStyle.Render(m.errText) + "\n")
	}
	if !m.hasOptions() {
		prefix := "Ctrl+D 제출 · "
		if m.custom {
			prefix += "Esc 선택지 복귀 · "
		}
		b.WriteString(helpStyle.Render(prefix +
			" /clarify 질문이 이해 안 됨 · /explain 설명 요청 · /skip 현재 개념 전체 미루기 · /object 이의 · /abort 중단"))
	} else {
		prefix := ""
		if q.Kind != engine.KindSessionLimit {
			prefix = "i 다른 의견 직접 입력 · "
		}
		b.WriteString(helpStyle.Render(prefix +
			"c 질문 명확화 · e 설명 · s 현재 개념 전체 미루기 · o 이의 · a 세션 중단 · Ctrl+C TUI 종료"))
	}
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
	p := tea.NewProgram(newModel(x), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return 1, err
	}
	return 0, nil
}

// RunStore waits for initialization and follows the active session pointer.
func RunStore(st *store.Store) (int, error) {
	m := newModel(nil)
	m.store = st
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return 1, err
	}
	return 0, nil
}

func (m model) questionWidth() int {
	if m.width <= 0 {
		return 76
	}
	return max(4, m.width-4)
}
