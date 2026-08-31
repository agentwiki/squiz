package engine

import "testing"

// Regression tests for the pre-release review findings.

// A --stage flag conflicting with --kind must be rejected: it previously
// bypassed the deferred-transfer lock and stage-already-passed checks.
func TestLadderStageKindMismatchRejected(t *testing.T) {
	s := newAISession(t, 2)
	_, err := s.Ask(AskParams{Kind: KindPredict, ConceptID: "c1", Stage: StageTransfer, Text: "p?"})
	wantErr(t, err, "conflicts")
}

// Retracting an unrelated concept must not clear another concept's
// retract obligation (I6).
func TestRetractWrongConceptKeepsObligation(t *testing.T) {
	s := newAISession(t, 2)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit})
	if err := s.SetVerify(VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: false}); err != nil {
		t.Fatal(err)
	}
	// c1's claim lost; retracting c2 instead must be refused
	err := s.Retract("c2", "claim", "")
	wantErr(t, err, "c1")
	if err := s.Retract("c1", "claim", ""); err != nil {
		t.Fatal(err)
	}
}

// A restored judgment referencing a version that is later retracted must be
// tainted again (I5): stages cannot stay "pass" on withdrawn claims.
func TestSecondRetractTaintsRestoredJudgment(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit})
	s.SetVerify(VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: false})
	s.Retract("c1", "claim", "")
	qid := s.PendingRestore[0]
	if err := s.Restore(Judgment{QID: qid, Alignment: Aligned, Support: SupMechanism,
		ModelClarity: ClarityExplicit, QuestionQuality: QValid}); err != nil {
		t.Fatal(err)
	}
	if s.Concept("c1").stage(StagePredict).State != StagePass {
		t.Fatal("restored pass expected")
	}
	// second retract of the (new) claim version
	if err := s.Retract("c1", "claim", ""); err != nil {
		t.Fatal(err)
	}
	if len(s.PendingRestore) == 0 {
		t.Fatal("second retract must taint the restored judgment (I5)")
	}
	if s.Concept("c1").stage(StagePredict).State == StagePass {
		t.Fatal("stage must not stay pass on a withdrawn claim version")
	}
}

// Restores spanning several concepts must recompute each concept's stages,
// not only the last one.
func TestRestoreRecomputesPerConcept(t *testing.T) {
	s := newAISession(t, 2)
	passCore(t, s, "c1")
	// force taints on c1 via defect (claim co-fix)
	if err := s.Defect("c1", "bug in dedup", []string{"prop"}); err != nil {
		t.Fatal(err)
	}
	if s.Concept("c1").stage(StagePredict).State == StagePass {
		t.Fatal("taint should downgrade c1 stages")
	}
	n := len(s.PendingRestore)
	if n == 0 {
		t.Fatal("defect must queue restores")
	}
	for _, qid := range append([]string(nil), s.PendingRestore...) {
		if err := s.Restore(Judgment{QID: qid, Alignment: Aligned, Support: SupMechanism,
			ModelClarity: ClarityExplicit, QuestionQuality: QValid}); err != nil {
			t.Fatal(err)
		}
	}
	for _, st := range Ladder[:3] {
		if s.Concept("c1").stage(st).State != StagePass {
			t.Fatalf("c1.%s must be recomputed to pass after restores", st)
		}
	}
}

// I2: while pending_restore is non-empty, judge/feedback/episode_close/
// skip/close are all blocked — only restore is allowed.
func TestPendingRestoreBlocksEverything(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Divergent})
	if err := s.Defect("c1", "bug", []string{"prop"}); err != nil {
		t.Fatal(err)
	}
	if len(s.PendingRestore) == 0 {
		t.Fatal("setup: restores queued")
	}
	wantErr(t, s.Feedback(FeedbackConcept, "c1", "x"), "I2")
	wantErr(t, s.EpisodeClose(CauseUndetermined, nil), "I2")
	wantErr(t, s.SkipConcept("c1"), "I2")
	_, err := s.Close()
	wantErr(t, err, "I2")
	err = s.Judge(Judgment{QID: "q9", Alignment: Aligned, QuestionQuality: QValid})
	wantErr(t, err, "I2")
}

// Discarding the post-explanation new-case question must re-arm the P1
// gate instead of silently dropping the requirement.
func TestNewCaseDiscardRearmsGate(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, DontKnow,
		Judgment{Alignment: Unknown})
	q := mustAsk(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerExplain})
	s.ExplanationRecord("c1", "explanation")
	aaj(t, s, AskParams{Kind: KindOwnWords, ConceptID: "c1", Text: "own?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	// the new-case question dies via objection discard
	nq := mustAsk(t, s, AskParams{Kind: KindBoundary, ConceptID: "c1", Text: "b?", NewCase: true})
	mustAnswer(t, s, Answer{QID: nq.QID, Action: AnswerObject})
	if err := s.ResolveObjection(true, ""); err != nil {
		t.Fatal(err)
	}
	if !s.AwaitNewCase {
		t.Fatal("discarded new case must re-arm AwaitNewCase (P1)")
	}
	// a plain ladder ask is still blocked
	_, err := s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "new-case")
}

// The close report from the engine must be exposed with design §14 items.
func TestFinalReportExposed(t *testing.T) {
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	s.Finalize("c1")
	rq := mustAsk(t, s, AskParams{Kind: KindReexplain, Text: "again"})
	mustAnswer(t, s, Answer{QID: rq.QID, Action: AnswerText, Text: "re-explained"})
	rep, err := s.Close()
	if err != nil {
		t.Fatal(err)
	}
	if s.FinalReport() != rep {
		t.Fatal("FinalReport must expose the close report")
	}
	if rep.SharedBlindSpotNote == "" || rep.ReexplainText != "re-explained" {
		t.Fatal("report must carry blind-spot note and reexplain text (design §14)")
	}
}
