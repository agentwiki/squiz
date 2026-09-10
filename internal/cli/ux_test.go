package cli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/agentwiki/squiz/internal/engine"
	"github.com/agentwiki/squiz/internal/ipc"
)

func TestNoticeSurvivesWaitReplayAndLegacyScreen(t *testing.T) {
	t.Chdir(t.TempDir())
	if code := Main([]string{"init", "--source", "concept"}); code != 0 {
		t.Fatal(code)
	}
	for _, args := range [][]string{{"concept", "add", "--name", "범위", "--claim", "가설"}, {"verify", "--concept", "c1", "--status", "contested"}, {"start"}, {"ask", "--kind", "open", "--text", "질문", "--notice", "사라지면 안 되는 조건"}} {
		if code := Main(args); code != 0 {
			t.Fatal(args, code)
		}
	}
	st, s, err := loadActive()
	if err != nil {
		t.Fatal(err)
	}
	x, _ := ipc.New(st.QueueDir(s.ID))
	if s.Pending.Notice != "사라지면 안 되는 조건" {
		t.Fatal("notice missing in replay")
	}
	if err := x.ClearQuestion(); err != nil {
		t.Fatal(err)
	}
	if code := Main([]string{"wait", "--timeout", "1ms"}); code != 3 {
		t.Fatal(code)
	}
	sc, err := x.ReadQuestion()
	if err != nil || sc.Notice != s.Pending.Notice {
		t.Fatalf("recovery: %#v %v", sc, err)
	}
	// Create a genuine legacy ask event (no notice field in AskParams).
	if err := x.PublishResponse(engine.Answer{QID: s.Pending.QID, Action: engine.AnswerChoice, Choice: 99}); err != nil {
		t.Fatal(err)
	}
	// A rejected response preserves the screen and is visible as an error.
	if code := Main([]string{"wait", "--timeout", "1ms"}); code != 5 {
		t.Fatal(code)
	}
	status, _ := x.ReadStatus()
	if status.State != "error" || !strings.Contains(status.Text, "다음 동작") {
		t.Fatalf("%#v", status)
	}
	if err := x.PublishResponse(engine.Answer{QID: s.Pending.QID, Action: engine.AnswerAbort}); err != nil {
		t.Fatal(err)
	}
	if code := Main([]string{"wait"}); code != 4 {
		t.Fatal(code)
	}
	status, _ = x.ReadStatus()
	if status.State != "aborted" {
		t.Fatalf("%#v", status)
	}
	_, s, err = loadActive()
	if err != nil {
		t.Fatal(err)
	}
	if ipc.StatusFor(s).State != "aborted" {
		t.Fatal("reconnect lost abort")
	}

	// Legacy sessions still retain an existing notice through wait.
	if code := Main([]string{"init", "--source", "concept"}); code != 0 {
		t.Fatal(code)
	}
	st, s, _ = loadActive()
	for _, ev := range []struct {
		typ string
		p   any
	}{
		{engine.EvConceptAdd, engine.ConceptAddPayload{ConceptParams: engine.ConceptParams{Name: "legacy", ClaimText: "claim", ClaimType: engine.ClaimFact}}},
		{engine.EvVerify, engine.VerifyParams{ConceptID: "c1", Status: engine.VerifyContested}},
		{engine.EvStart, struct{}{}}, {engine.EvAsk, engine.AskParams{Kind: engine.KindOpen, Text: "legacy question"}},
	} {
		if err := st.Commit(s, ev.typ, ev.p); err != nil {
			t.Fatal(err)
		}
	}
	x, _ = ipc.New(st.QueueDir(s.ID))
	if err := x.PublishQuestion(screenFor(s, s.Pending, "legacy notice")); err != nil {
		t.Fatal(err)
	}
	if code := Main([]string{"wait", "--timeout", "1ms"}); code != 3 {
		t.Fatal(code)
	}
	sc, _ = x.ReadQuestion()
	if sc.Notice != "legacy notice" {
		t.Fatal(sc)
	}
	if _, err := os.Stat(x.Dir); errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
