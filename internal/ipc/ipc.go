// Package ipc is the file-based question/response exchange between the CLI
// (AI side) and the TUI (user side): squiz-core.md §4-§5. Both sides only
// ever publish via temp-file + rename, so a reader either sees a complete
// JSON document or nothing (I4 atomicity).
package ipc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/agentwiki/squiz/internal/engine"
)

const (
	questionFile = "question.json"
	responseFile = "response.json"
)

// Screen is what the TUI renders: the pending question plus header context
// (A7: concept and stage always visible) and an optional non-question
// notice (not counted in burden).
type Screen struct {
	Question    *engine.Question `json:"question"`
	ConceptName string           `json:"concept_name,omitempty"`
	Stage       engine.Stage     `json:"stage,omitempty"`
	Notice      string           `json:"notice,omitempty"`
	JudgedCount int              `json:"judged_count"`
	BurdenCount int              `json:"burden_count"`
}

type Exchange struct{ Dir string }

func New(dir string) (*Exchange, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Exchange{Dir: dir}, nil
}

func (x *Exchange) qPath() string { return filepath.Join(x.Dir, questionFile) }
func (x *Exchange) rPath() string { return filepath.Join(x.Dir, responseFile) }

// PublishQuestion posts the screen for the TUI and clears stale responses.
func (x *Exchange) PublishQuestion(sc Screen) error {
	_ = os.Remove(x.rPath())
	return atomicWrite(x.qPath(), sc)
}

// ClearQuestion removes the posted question (after the response is consumed).
func (x *Exchange) ClearQuestion() error {
	err := os.Remove(x.qPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadQuestion returns the current screen, or nil if none is posted.
func (x *Exchange) ReadQuestion() (*Screen, error) {
	b, err := os.ReadFile(x.qPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sc Screen
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

// PublishResponse is called by the TUI when the user acts.
func (x *Exchange) PublishResponse(a engine.Answer) error {
	return atomicWrite(x.rPath(), a)
}

// RefreshQuestion republishes the question WITHOUT clearing any response
// already posted by the TUI (used by `squiz wait` to heal a stale
// question.json after a crash between commit and clear).
func (x *Exchange) RefreshQuestion(sc Screen) error {
	return atomicWrite(x.qPath(), sc)
}

// PeekResponse reads a response without consuming it (nil if none yet).
// The caller consumes it with DropResponse only after the engine approved
// it (squiz-core.md §2: approval before consumption).
func (x *Exchange) PeekResponse() (*engine.Answer, error) {
	b, err := os.ReadFile(x.rPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a engine.Answer
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// DropResponse removes the posted response file.
func (x *Exchange) DropResponse() error {
	err := os.Remove(x.rPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Wait polls for a response until timeout (0 = wait forever). The response
// is NOT consumed; see PeekResponse.
func (x *Exchange) Wait(timeout time.Duration) (*engine.Answer, error) {
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		a, err := x.PeekResponse()
		if err != nil {
			return nil, err
		}
		if a != nil {
			return a, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, os.ErrDeadlineExceeded
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func atomicWrite(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Status carries durable session progress independently of the pending question.
type Status struct {
	Seq      int      `json:"seq"`
	State    string   `json:"state"`
	Text     string   `json:"text"`
	Messages []string `json:"messages,omitempty"`
}

func (x *Exchange) PublishStatus(s Status) error {
	return atomicWrite(filepath.Join(x.Dir, "status.json"), s)
}
func (x *Exchange) ReadStatus() (*Status, error) {
	b, err := os.ReadFile(filepath.Join(x.Dir, "status.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func StatusFor(s *engine.Session) Status {
	st := Status{Seq: s.Seq, State: "working", Text: "AI가 응답을 처리 중입니다. 오래 멈추면 AI 터미널에서 진행 상태를 확인하세요.", Messages: s.LearnerMessages}
	if s.Pending != nil {
		st.State = "question"
		st.Text = "답변을 기다립니다."
	}
	if s.Phase == engine.PhasePrep {
		st.Text = "세션에 연결했습니다. AI가 첫 질문을 준비 중입니다."
	}
	if s.Aborted {
		st.State = "aborted"
		st.Text = "세션을 중단했습니다. Ctrl+C로 화면을 닫거나 새 세션을 기다리세요."
	}
	if s.Phase == engine.PhaseDone {
		st.State = "done"
		st.Text = "세션이 완료되었습니다. Ctrl+C로 화면을 닫거나 새 세션을 기다리세요."
		if s.Aborted {
			st.State = "aborted"
			st.Text = "세션을 중단했습니다. Ctrl+C로 화면을 닫거나 새 세션을 기다리세요."
		}
		if r := s.FinalReport(); r != nil {
			for _, c := range r.Concepts {
				st.Text += "\n" + c.Name + ": " + c.Phrase
			}
			if r.ReexplainText != "" {
				st.Text += "\n마지막 정리: " + r.ReexplainText
			}
			st.Text += "\n" + r.RecheckAdvice
		}
	}
	return st
}
