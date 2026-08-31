package store

import (
	"encoding/json"
	"testing"

	"github.com/agentwiki/squiz/internal/engine"
)

// TestReplayDeterminism: events.jsonl replay must reproduce the exact state
// (docs/spec/core.md §4: the log is the source of truth, state.json a cache).
func TestReplayDeterminism(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.Create(engine.SourceAI, engine.RoleNone)
	if err != nil {
		t.Fatal(err)
	}
	commit := func(typ string, payload any) {
		t.Helper()
		if err := st.Commit(s, typ, payload); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}
	commit(engine.EvConceptAdd, engine.ConceptAddPayload{
		ConceptParams: engine.ConceptParams{Name: "idempotency", ClaimText: "same key -> one record"},
	})
	commit(engine.EvVerify, engine.VerifyParams{
		ConceptID: "c1", Status: engine.VerifySupported,
		Evidence: []engine.Evidence{{Kind: "execution", Text: "two calls, one record", Excludes: "double charge"}},
	})
	commit(engine.EvStart, struct{}{})
	commit(engine.EvAsk, engine.AskParams{Kind: engine.KindOpen, Text: "explain"})
	commit(engine.EvAnswer, engine.Answer{QID: "q1", Action: engine.AnswerText, Text: "baseline"})
	commit(engine.EvAsk, engine.AskParams{Kind: engine.KindPredict, ConceptID: "c1", Text: "predict?"})
	commit(engine.EvAnswer, engine.Answer{QID: "q2", Action: engine.AnswerText, Text: "two records", Confidence: engine.Sure})
	commit(engine.EvJudge, engine.Judgment{QID: "q2", Alignment: engine.Contradicted,
		ModelClarity: engine.ClarityExplicit, QuestionQuality: engine.QValid, Confidence: engine.Sure})
	commit(engine.EvVerify, engine.VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: true,
		Evidence: []engine.Evidence{{Kind: "execution", Text: "re-run"}}})
	commit(engine.EvFeedback, engine.FeedbackPayload{Kind: engine.FeedbackRecheck, Text: "confirmed"})

	// a rejected event must leave no trace
	if err := st.Commit(s, engine.EvAsk, engine.AskParams{Kind: engine.KindWhy, ConceptID: "c1", Text: "w?"}); err == nil {
		t.Fatal("why during open episode should be rejected")
	}

	loaded, err := st.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(s)
	b, _ := json.Marshal(loaded)
	if string(a) != string(b) {
		t.Fatalf("replay diverged:\nlive:   %s\nreplay: %s", a, b)
	}
	if loaded.Seq != s.Seq {
		t.Fatalf("seq %d != %d", loaded.Seq, s.Seq)
	}
	// live session was mutated by the rejected Apply before the error? It
	// must not have been persisted: replay must still reject the same ask.
	if _, err := loaded.Ask(engine.AskParams{Kind: engine.KindWhy, ConceptID: "c1", Text: "w?"}); err == nil {
		t.Fatal("state after replay should still reject the ladder ask (episode open)")
	}
}
