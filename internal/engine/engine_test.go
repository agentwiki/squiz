package engine

import (
	"errors"
	"strings"
	"testing"
)

// ---- helpers ---------------------------------------------------------------

func newAISession(t *testing.T, nConcepts int) *Session {
	t.Helper()
	s, err := NewSession("t1", SourceAI, RoleNone)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < nConcepts; i++ {
		name := string(rune('A' + i))
		if _, err := s.AddConcept(ConceptParams{
			Name: "concept " + name, ClaimText: "claim " + name, ClaimType: ClaimBehavior,
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range s.Concepts {
		if err := s.SetVerify(VerifyParams{
			ConceptID: c.ID, Status: VerifySupported,
			Evidence: []Evidence{{Kind: "execution", Text: "counterexample run", Excludes: "naive model"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	// open baseline
	q := mustAsk(t, s, AskParams{Kind: KindOpen, Text: "explain freely"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "baseline", Confidence: Unsure})
	if s.Phase != PhaseLadder {
		t.Fatalf("phase = %s, want ladder", s.Phase)
	}
	return s
}

func mustAsk(t *testing.T, s *Session, p AskParams) *Question {
	t.Helper()
	q, err := s.Ask(p)
	if err != nil {
		t.Fatalf("ask %s: %v", p.Kind, err)
	}
	return q
}

func mustAnswer(t *testing.T, s *Session, a Answer) int {
	t.Helper()
	code, err := s.AnswerQuestion(a)
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	return code
}

func mustJudge(t *testing.T, s *Session, j Judgment) {
	t.Helper()
	if err := s.Judge(j); err != nil {
		t.Fatalf("judge: %v", err)
	}
}

// askAnswerJudge runs one full screen: ask -> text answer -> judgment.
func aaj(t *testing.T, s *Session, p AskParams, conf Confidence, j Judgment) {
	t.Helper()
	q := mustAsk(t, s, p)
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "answer", Confidence: conf})
	j.QID = q.QID
	if j.QuestionQuality == "" {
		j.QuestionQuality = QValid
	}
	if j.Confidence == "" {
		j.Confidence = conf
	}
	mustJudge(t, s, j)
}

func alignedMech() Judgment {
	return Judgment{Alignment: Aligned, Support: SupMechanism, ModelClarity: ClarityExplicit}
}

// passLadder drives predict/why/boundary of concept c to pass.
func passCore(t *testing.T, s *Session, cid string) {
	t.Helper()
	for _, k := range []QuestionKind{KindPredict, KindWhy, KindBoundary} {
		aaj(t, s, AskParams{Kind: k, ConceptID: cid, Text: string(k) + "?"}, Sure, alignedMech())
	}
}

func wantErr(t *testing.T, err error, frag string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", frag)
	}
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("expected ErrNotAllowed, got %v", err)
	}
	if frag != "" && !strings.Contains(err.Error(), frag) {
		t.Fatalf("error %q does not contain %q", err, frag)
	}
}

// ---- transitions §1 session phases -----------------------------------------------------

func TestStartBlockedByPendingVerify(t *testing.T) {
	s, _ := NewSession("t", SourceAI, RoleNone)
	s.AddConcept(ConceptParams{Name: "a", ClaimText: "c"}, false)
	err := s.Start()
	wantErr(t, err, "verify=pending")
}

func TestConceptModeSupportedNeedsExternalRefs(t *testing.T) {
	s, _ := NewSession("t", SourceConcept, RoleNone)
	c, _ := s.AddConcept(ConceptParams{Name: "a", ClaimText: "c"}, false)
	err := s.SetVerify(VerifyParams{ConceptID: c.ID, Status: VerifySupported})
	wantErr(t, err, "external refs")
}

func TestTransferDeferredAcrossConcepts(t *testing.T) {
	s := newAISession(t, 2)
	passCore(t, s, "c1")
	// A.transfer locked until B.boundary
	_, err := s.Ask(AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"})
	wantErr(t, err, "deferred")
	passCore(t, s, "c2")
	if !s.Concept("c1").TransferUnlocked {
		t.Fatal("c1 transfer should unlock after c2 boundary")
	}
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	if s.Concept("c1").stage(StageTransfer).State != StagePass {
		t.Fatal("c1 transfer should pass")
	}
}

func TestSingleConceptTransferGapNote(t *testing.T) {
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	if !s.Concept("c1").TransferUnlocked {
		t.Fatal("single concept: transfer unlocks at boundary")
	}
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	if !s.Concept("c1").Outcome.TransferGapNote {
		t.Fatal("single-concept outcome must carry the gap note")
	}
}

func mustFeedbackConcept(t *testing.T, s *Session, cid string) {
	t.Helper()
	if err := s.Feedback(FeedbackConcept, cid, "confirmed/range/open"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrateNeedsTwoDistinctConcepts(t *testing.T) {
	// single demonstrated concept must NOT enter integrate
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	if s.Phase != PhaseClose {
		t.Fatalf("one concept: phase = %s, want close (no integrate)", s.Phase)
	}
}

func TestIntegrateEntry(t *testing.T) {
	s := newAISession(t, 2)
	passCore(t, s, "c1")
	passCore(t, s, "c2")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c2", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	mustFeedbackConcept(t, s, "c2")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize("c2"); err != nil {
		t.Fatal(err)
	}
	if s.Phase != PhaseIntegrate {
		t.Fatalf("phase = %s, want integrate (c1 demonstrated + c2 why pass)", s.Phase)
	}
	// integrate judgments cap at 2, then close
	aaj(t, s, AskParams{Kind: KindIntegrate, Text: "relate A and B"}, Sure, alignedMech())
	aaj(t, s, AskParams{Kind: KindIntegrate, Text: "relate more"}, Sure, alignedMech())
	if s.Phase != PhaseClose {
		t.Fatalf("phase = %s, want close after 2 integrate judgments", s.Phase)
	}
}

// ---- P1 scaffold fading ----------------------------------------------------

func TestScaffoldFadingChain(t *testing.T) {
	s := newAISession(t, 1)
	// predict contradicted (unsure, vague) -> episode
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, Support: SupNone, ModelClarity: ClarityVague})
	if s.Episode == nil || !s.Episode.Open {
		t.Fatal("episode should open")
	}
	// narrow aligned -> recombine mandatory
	aaj(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	_, err := s.Ask(AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n2?"})
	wantErr(t, err, "recombine")
	// recombine aligned -> retry mandatory
	aaj(t, s, AskParams{Kind: KindRecombine, ConceptID: "c1", Text: "r?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	_, err = s.Ask(AskParams{Kind: KindHint, ConceptID: "c1", Text: "h?", HintLevel: 2})
	wantErr(t, err, "retry")
	// unscaffolded retry pass -> stage pass, path guided
	aaj(t, s, AskParams{Kind: KindRetry, ConceptID: "c1", Text: "fresh case?"}, Sure, alignedMech())
	si := s.Concept("c1").stage(StagePredict)
	if si.State != StagePass || si.Path != PathGuided {
		t.Fatalf("predict = %s/%s, want pass/guided", si.State, si.Path)
	}
	// ladder blocked until episode close
	_, err = s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "episode")
	// user_misconception requires the full checklist (P3)
	err = s.EpisodeClose(CauseUserMisconception, &MisconceptionChecklist{QuestionValid: true})
	wantErr(t, err, "checklist")
	ok := &MisconceptionChecklist{true, true, true, true, true, true}
	if err := s.EpisodeClose(CauseUserMisconception, ok); err != nil {
		t.Fatal(err)
	}
	if s.Concept("c1").MisconceptionEpisodes != 1 {
		t.Fatal("misconception episode should count")
	}
	// ladder resumes
	aaj(t, s, AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"}, Sure, alignedMech())
}

func TestEpisodeCauseDefaultsUndetermined(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Unknown})
	if err := s.EpisodeClose("", nil); err != nil {
		t.Fatal(err)
	}
	if s.Episode.Cause != CauseUndetermined {
		t.Fatalf("cause = %s, want undetermined", s.Episode.Cause)
	}
	if s.Concept("c1").MisconceptionEpisodes != 0 {
		t.Fatal("undetermined must not count as misconception")
	}
}

func TestScaffoldCapThree(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	for i := 0; i < 3; i++ {
		aaj(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"}, Unsure,
			Judgment{Alignment: Partial, PartialCredit: "some part"})
	}
	_, err := s.Ask(AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n4?"})
	wantErr(t, err, "scaffold limit")
}

func TestRetryCap(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	aaj(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	aaj(t, s, AskParams{Kind: KindRecombine, ConceptID: "c1", Text: "r?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	// two failed retries exhaust the cap
	aaj(t, s, AskParams{Kind: KindRetry, ConceptID: "c1", Text: "r1?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	aaj(t, s, AskParams{Kind: KindRetry, ConceptID: "c1", Text: "r2?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	_, err := s.Ask(AskParams{Kind: KindRetry, ConceptID: "c1", Text: "r3?"})
	wantErr(t, err, "retry cap")
	// explanation is the sanctioned exit
	if err := s.ExplanationRecord("c1", "here is how it works"); err != nil {
		t.Fatal(err)
	}
}

func TestPassRequiresUnscaffolded(t *testing.T) {
	// a scaffolded aligned answer at the stage kind must NOT pass directly:
	// inside an episode, ladder kinds are blocked entirely; only retry passes.
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	aaj(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	_, err := s.Ask(AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p again?"})
	wantErr(t, err, "")
}

// ---- P7 / I7 / transitions §6 question quality ----------------------------------------

func TestPartialRequiresCredit(t *testing.T) {
	s := newAISession(t, 1)
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "a", Confidence: Unsure})
	err := s.Judge(Judgment{QID: q.QID, Alignment: Partial, QuestionQuality: QValid})
	wantErr(t, err, "partial_credit")
	// judge with credit succeeds
	mustJudge(t, s, Judgment{QID: q.QID, Alignment: Partial, PartialCredit: "the key part", QuestionQuality: QValid})
}

func TestInvalidQuestionNotJudged(t *testing.T) {
	s := newAISession(t, 1)
	j0, b0 := s.JudgedCount, s.BurdenCount
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "vague?"}, Sure,
		Judgment{Alignment: Contradicted, QuestionQuality: QUnderspecified})
	if s.JudgedCount != j0 {
		t.Fatal("invalid question must not count as judged (I7)")
	}
	if s.BurdenCount != b0+1 {
		t.Fatal("invalid question still counts as burden (P8)")
	}
	if s.Episode != nil && s.Episode.Open {
		t.Fatal("invalid question must not open an episode")
	}
	if s.Concept("c1").stage(StagePredict).State != StagePending {
		t.Fatal("invalid question must not move the stage")
	}
	if s.NeedRecheck {
		t.Fatal("invalid question must not trigger recheck")
	}
}

func TestTwoLeadingQuestionsOpenInvestigation(t *testing.T) {
	s := newAISession(t, 1)
	for i := 0; i < 2; i++ {
		aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "leading?"}, Sure,
			Judgment{Alignment: Aligned, QuestionQuality: QLeading})
	}
	if s.openInvestigation() == nil {
		t.Fatal("two leading questions on one concept must open an investigation")
	}
}

func TestPrerequisiteGap(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, QuestionQuality: QPrereqMissing})
	if !s.Concept("c1").Paused {
		t.Fatal("prerequisite_missing pauses the downstream concept")
	}
	_, err := s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "paused")
	// provisional registration (depth cap 1)
	pc, err := s.AddConcept(ConceptParams{Name: "prereq", ClaimText: "base claim"}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddConcept(ConceptParams{Name: "prereq2", ClaimText: "deeper"}, true)
	wantErr(t, err, "depth cap")
	// recovery question counts in burden only
	j0, b0 := s.JudgedCount, s.BurdenCount
	aaj(t, s, AskParams{Kind: KindPrereq, ConceptID: pc.ID, Text: "recover?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	if s.JudgedCount != j0 || s.BurdenCount != b0+1 {
		t.Fatalf("prereq recovery: judged %d->%d burden %d->%d, want judged unchanged, burden+1",
			j0, s.JudgedCount, b0, s.BurdenCount)
	}
	if s.Concept("c1").Paused {
		t.Fatal("aligned recovery unpauses downstream")
	}
}

// ---- I6 / P6 / transitions §5 AI error paths -------------------------------------------

func TestSureContradictionForcesRecheck(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit})
	if !s.NeedRecheck {
		t.Fatal("contradicted+sure must force recheck (I6)")
	}
	_, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: "c1", Text: "c?", EvidenceRef: "e1"})
	wantErr(t, err, "recheck")
	// recheck supports the claim -> feedback gate (P6)
	if err := s.SetVerify(VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: true,
		Evidence: []Evidence{{Kind: "execution", Text: "second run", Excludes: "user model"}}}); err != nil {
		t.Fatal(err)
	}
	if !s.NeedRecheckFeedback {
		t.Fatal("recheck completion requires feedback (P6)")
	}
	_, err = s.Ask(AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	wantErr(t, err, "feedback")
	if err := s.Feedback(FeedbackRecheck, "", "confirmed: one record, cached response"); err != nil {
		t.Fatal(err)
	}
	// now consequence is available (M=explicit survived, verified evidence)
	ev := s.Concept("c1").Evidence[0].ID
	if _, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: "c1", Text: "which assumption?", EvidenceRef: ev}); err != nil {
		t.Fatalf("consequence should now be allowed: %v", err)
	}
}

func TestRecheckRefutedForcesRetractRestore(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit})
	if err := s.SetVerify(VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: false}); err != nil {
		t.Fatal(err)
	}
	// claim lost: only retract is allowed
	_, err := s.Ask(AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	wantErr(t, err, "retract")
	if err := s.Retract("c1", "claim", ""); err != nil {
		t.Fatal(err)
	}
	if len(s.PendingRestore) == 0 {
		t.Fatal("retraction must taint and queue dependent judgments")
	}
	// I2: restore only
	_, err = s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "restore")
	qid := s.PendingRestore[0]
	if err := s.Restore(Judgment{QID: qid, Alignment: Aligned, Support: SupMechanism,
		ModelClarity: ClarityExplicit, QuestionQuality: QValid}); err != nil {
		t.Fatal(err)
	}
	if len(s.PendingRestore) != 0 {
		t.Fatal("restore should drain the queue")
	}
	// the re-judged answer now passes predict (retroactive re-judgment)
	if s.Concept("c1").stage(StagePredict).State != StagePass {
		t.Fatal("restored aligned judgment should recompute predict=pass")
	}
	if !s.Concept("c1").TaintContrib {
		t.Fatal("taint contribution must be recorded for consistency")
	}
}

func TestDefectVindicationOnly(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Sure,
		Judgment{Alignment: Divergent})
	if s.openInvestigation() == nil {
		t.Fatal("divergent opens an investigation")
	}
	err := s.Defect("c1", "off-by-one in dedup", nil)
	wantErr(t, err, "vindicated")
	if err := s.Defect("c1", "off-by-one in dedup", []string{"second request creates a new record"}); err != nil {
		t.Fatal(err)
	}
	c := s.Concept("c1")
	if c.stage(StagePredict).State == StagePass {
		t.Fatal("defect must not pass any stage")
	}
	if len(c.Vindicated) != 1 {
		t.Fatal("vindicated proposition must be recorded")
	}
	if len(s.PendingRestore) == 0 {
		t.Fatal("defect invalidates dependents")
	}
}

func TestConsequenceRequiresVerifiedEvidence(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit})
	_, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: "c1", Text: "c?"})
	wantErr(t, err, "evidence")
	ev := s.Concept("c1").Evidence[0].ID
	if _, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: "c1", Text: "c?", EvidenceRef: ev}); err != nil {
		t.Fatal(err)
	}
}

func TestConsequenceBannedWithoutExplicitModel(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	ev := s.Concept("c1").Evidence[0].ID
	_, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: "c1", Text: "c?", EvidenceRef: ev})
	wantErr(t, err, "explicit_prediction")
}

func TestAuthorWhyCannotBeContradicted(t *testing.T) {
	s, _ := NewSession("t", SourceCode, RoleAuthor)
	c, _ := s.AddConcept(ConceptParams{Name: "a", ClaimText: "c", ClaimType: ClaimIntent}, false)
	s.SetVerify(VerifyParams{ConceptID: c.ID, Status: VerifySupported,
		Evidence: []Evidence{{Kind: "execution", Text: "run"}}})
	s.Start()
	q := mustAsk(t, s, AskParams{Kind: KindOpen, Text: "your three decisions?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "…"})
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: c.ID, Text: "p?"}, Sure, alignedMech())
	q = mustAsk(t, s, AskParams{Kind: KindWhy, ConceptID: c.ID, Text: "intent?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "a", Confidence: Sure})
	err := s.Judge(Judgment{QID: q.QID, Alignment: Contradicted, QuestionQuality: QValid})
	wantErr(t, err, "author")
	// mismatch is the sanctioned route: why passes with intent recorded
	mustJudge(t, s, Judgment{QID: q.QID, Alignment: Mismatch, QuestionQuality: QValid})
	if s.Concept(c.ID).stage(StageWhy).State != StagePass {
		t.Fatal("mismatch records why=pass with intent")
	}
}

// ---- transitions §4 explanation & autonomy ---------------------------------------------

func TestExplanationChainAndLadderResume(t *testing.T) {
	s := newAISession(t, 1)
	// fail predict into an episode, then user requests explanation (exit 6)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, DontKnow,
		Judgment{Alignment: Unknown})
	q := mustAsk(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	code := mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerExplain, Reason: "lost"})
	if code != 6 {
		t.Fatalf("explain exit code = %d, want 6", code)
	}
	if err := s.ExplanationRecord("c1", "the mechanism is ..."); err != nil {
		t.Fatal(err)
	}
	// only own_words now (P1)
	_, err := s.Ask(AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	wantErr(t, err, "own_words")
	aaj(t, s, AskParams{Kind: KindOwnWords, ConceptID: "c1", Text: "in your words?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	// only new-case boundary/transfer now
	_, err = s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "new-case")
	_, err = s.Ask(AskParams{Kind: KindBoundary, ConceptID: "c1", Text: "b?"})
	wantErr(t, err, "new case")
	aaj(t, s, AskParams{Kind: KindBoundary, ConceptID: "c1", Text: "new case b?", NewCase: true}, Sure,
		alignedMech())
	c := s.Concept("c1")
	if c.stage(StageBoundary).State != StagePass || c.stage(StageBoundary).Path != PathAfterExplanation {
		t.Fatalf("new-case boundary = %s/%s, want pass/after_explanation",
			c.stage(StageBoundary).State, c.stage(StageBoundary).Path)
	}
	// ladder resumes after the new case — remaining stages still askable
	aaj(t, s, AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"}, Sure, alignedMech())
	if c.stage(StageWhy).State != StagePass {
		t.Fatal("ladder must resume after the new case")
	}
	// path is after_explanation at finalize
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p2?"}, Sure, alignedMech())
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	out := c.Outcome
	if out.AcquisitionPath != PathAfterExplanation {
		t.Fatalf("path = %s, want after_explanation", out.AcquisitionPath)
	}
	if out.LearningOutcome != OutcomeDemonstrated {
		t.Fatalf("outcome = %s, want demonstrated (all four passed)", out.LearningOutcome)
	}
	if !out.RecheckRecommended {
		t.Fatal("after_explanation implies recheck_recommended")
	}
}

func TestOwnWordsRepetitionGetsOneApplicationTask(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, DontKnow,
		Judgment{Alignment: Unknown})
	q := mustAsk(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerExplain})
	s.ExplanationRecord("c1", "explanation")
	// repetition (aligned but no mechanism reconstruction)
	aaj(t, s, AskParams{Kind: KindOwnWords, ConceptID: "c1", Text: "own words?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupConvention})
	if !s.AwaitOwnWords {
		t.Fatal("repetition -> one format-changed application task")
	}
	aaj(t, s, AskParams{Kind: KindOwnWords, ConceptID: "c1", Text: "apply it?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	if !s.AwaitNewCase {
		t.Fatal("reconstruction -> new case required")
	}
}

func TestSkipIsDeferred(t *testing.T) {
	s := newAISession(t, 2)
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	code := mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerSkip})
	if code != 8 {
		t.Fatalf("skip exit = %d, want 8", code)
	}
	c := s.Concept("c1")
	if !c.Deferred || c.Outcome.LearningOutcome != OutcomeDeferred || c.Outcome.EndReason != EndSkipped {
		t.Fatalf("skip => deferred/skipped, got %+v", c.Outcome)
	}
	if s.CurrentConcept != "c2" {
		t.Fatal("current concept advances after skip")
	}
}

func TestExplanationRequestIsNotAnEndReason(t *testing.T) {
	s := newAISession(t, 1)
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, DontKnow,
		Judgment{Alignment: Unknown})
	q := mustAsk(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1", Text: "n?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerExplain})
	s.ExplanationRecord("c1", "explanation")
	aaj(t, s, AskParams{Kind: KindOwnWords, ConceptID: "c1", Text: "own?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?", NewCase: true}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	out := s.Concept("c1").Outcome
	if out.EndReason != EndCompleted {
		t.Fatalf("end_reason = %s, want completed (P5: help is not a penalty)", out.EndReason)
	}
	found := false
	for _, ev := range out.SupportEvents {
		if ev == "explanation_requested" {
			found = true
		}
	}
	if !found {
		t.Fatal("explanation_requested must be recorded as a support event")
	}
}

func TestClarifyEscalation(t *testing.T) {
	s := newAISession(t, 1)
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	if code := mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerClarify}); code != 2 {
		t.Fatal("clarify exit 2")
	}
	q2 := mustAsk(t, s, AskParams{Kind: KindClarifyReply, ConceptID: "c1", Text: "reworded?"})
	mustAnswer(t, s, Answer{QID: q2.QID, Action: AnswerClarify})
	q3 := mustAsk(t, s, AskParams{Kind: KindClarifyReply, ConceptID: "c1", Text: "as a concrete example?"})
	mustAnswer(t, s, Answer{QID: q3.QID, Action: AnswerClarify})
	// third clarify: discarded as inaccessible
	found := false
	for _, inc := range s.Concept("c1").Incidents {
		if inc.Type == "question_discarded" {
			found = true
		}
	}
	if !found {
		t.Fatal("third clarify discards the question (inaccessible)")
	}
	// a fresh question can now be asked
	if _, err := s.Ask(AskParams{Kind: KindPredict, ConceptID: "c1", Text: "different modality?"}); err != nil {
		t.Fatal(err)
	}
}

// ---- P8 limits --------------------------------------------------------------

func TestJudged30BlocksLearningQuestions(t *testing.T) {
	s := newAISession(t, 1)
	s.JudgedCount = 30
	_, err := s.Ask(AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	wantErr(t, err, "30")
	// non-learning operations stay available (safe wrap-up)
	if err := s.Feedback(FeedbackConcept, "c1", "wrap"); err != nil {
		t.Fatal(err)
	}
}

func TestBurdenGateOffersChoice(t *testing.T) {
	s := newAISession(t, 1)
	s.BurdenCount = 29
	s.JudgedCount = 4
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	if !s.LimitChoicePending {
		t.Fatal("burden 30 > 1.5*4 must trip the limit choice")
	}
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "a", Confidence: Sure})
	mustJudge(t, s, Judgment{QID: q.QID, Alignment: Aligned, Support: SupMechanism,
		ModelClarity: ClarityExplicit, QuestionQuality: QValid})
	_, err := s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "choice")
	cq := mustAsk(t, s, AskParams{Kind: KindSessionLimit, Text: "continue?",
		Options: []string{"계속", "요약 후 종료", "설명 전환"}})
	mustAnswer(t, s, Answer{QID: cq.QID, Action: AnswerChoice, Choice: 2})
	if s.Phase != PhaseClose {
		t.Fatalf("summary choice: phase = %s, want close", s.Phase)
	}
}

// ---- transfer setback / why return -----------------------------------------

func TestTransferSetbackWhyReturn(t *testing.T) {
	s := newAISession(t, 2)
	passCore(t, s, "c1")
	passCore(t, s, "c2")
	// c1 transfer contradicted -> why return (no episode yet)
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	if s.Concept("c1").WhyReturns != 1 {
		t.Fatal("first transfer setback returns to why")
	}
	if s.Episode != nil && s.Episode.Open {
		t.Fatal("why return is not an episode")
	}
	// why re-confirmation is allowed even though why already passed
	aaj(t, s, AskParams{Kind: KindWhy, ConceptID: "c1", Text: "structure again?"}, Sure, alignedMech())
	// fresh transfer case; second contradiction opens an episode
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t2?"}, Unsure,
		Judgment{Alignment: Contradicted, ModelClarity: ClarityVague})
	if s.Episode == nil || !s.Episode.Open {
		t.Fatal("second transfer setback opens an episode")
	}
	// finalize later marks inconsistency
	s.EpisodeClose(CauseUndetermined, nil)
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	if s.Concept("c1").Outcome.SessionConsistency != "inconsistent" {
		t.Fatal("why return marks session_consistency=inconsistent")
	}
}

// ---- P2 retention -----------------------------------------------------------

func TestRetentionAlwaysUntested(t *testing.T) {
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	s.Finalize("c1")
	if s.Concept("c1").Outcome.Retention != "untested" {
		t.Fatal("retention is untested in-session, always (P2)")
	}
}

// ---- finalize gates ---------------------------------------------------------

func TestFinalizeNeedsConceptFeedback(t *testing.T) {
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1", Text: "t?"}, Sure, alignedMech())
	err := s.Finalize("c1")
	wantErr(t, err, "feedback")
}

func TestDemonstratedNeedsFourUnscaffolded(t *testing.T) {
	s := newAISession(t, 1)
	passCore(t, s, "c1")
	// transfer never attempted -> partial (3 passes)
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	if s.Concept("c1").Outcome.LearningOutcome != OutcomePartial {
		t.Fatalf("3 passes = partial, got %s", s.Concept("c1").Outcome.LearningOutcome)
	}
}

// ---- full trajectory (design §17) -------------------------------------------

func TestExampleTrajectory(t *testing.T) {
	s := newAISession(t, 2)
	// predict: contradicted / sure -> I6 recheck
	aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1",
		Text: "같은 키 두 번: 레코드 수와 두 번째 응답은?"}, Sure,
		Judgment{Alignment: Contradicted, Support: SupConvention, ModelClarity: ClarityExplicit})
	if err := s.SetVerify(VerifyParams{ConceptID: "c1", Recheck: true, RecheckSupportsClaim: true,
		Evidence: []Evidence{{Kind: "execution", Text: "같은 키 2회 → 레코드 1", Excludes: "두 번 결제"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Feedback(FeedbackRecheck, "", "확인된 실행: 레코드 1, 두 번째는 캐시"); err != nil {
		t.Fatal(err)
	}
	// consequence (episode 1)
	ev := s.Concept("c1").Evidence[len(s.Concept("c1").Evidence)-1].ID
	aaj(t, s, AskParams{Kind: KindConsequence, ConceptID: "c1",
		Text: "실행 결과와 갈리는 가정은?", EvidenceRef: ev}, Unsure,
		Judgment{Alignment: Partial, Support: SupMechanism, ModelClarity: ClarityVague,
			PartialCredit: "구분 여부가 핵심이라는 점"})
	aaj(t, s, AskParams{Kind: KindNarrow, ConceptID: "c1",
		Text: "요청에 같음을 알 정보가 있어?"}, Unsure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	aaj(t, s, AskParams{Kind: KindRecombine, ConceptID: "c1",
		Text: "그 키로 서버가 하는 일을 처음 질문에 다시 넣으면?"}, Sure,
		Judgment{Alignment: Aligned, Support: SupMechanism})
	aaj(t, s, AskParams{Kind: KindRetry, ConceptID: "c1",
		Text: "첫 요청이 도달 못 했으면?"}, Sure, alignedMech())
	if err := s.EpisodeClose(CauseUserMisconception,
		&MisconceptionChecklist{true, true, true, true, true, true}); err != nil {
		t.Fatal(err)
	}
	// why, boundary; then concept B; then A's transfer
	aaj(t, s, AskParams{Kind: KindWhy, ConceptID: "c1", Text: "왜 한 번만?"}, Sure, alignedMech())
	aaj(t, s, AskParams{Kind: KindBoundary, ConceptID: "c1", Text: "언제 깨져?"}, Sure, alignedMech())
	passCore(t, s, "c2")
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c1",
		Text: "이메일 발송에 옮기면?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c1")
	if err := s.Finalize("c1"); err != nil {
		t.Fatal(err)
	}
	out := s.Concept("c1").Outcome
	if out.LearningOutcome != OutcomeDemonstrated || out.AcquisitionPath != PathGuided {
		t.Fatalf("got %s/%s, want demonstrated/guided", out.LearningOutcome, out.AcquisitionPath)
	}
	if out.SessionConsistency != "consistent" {
		t.Fatalf("one misconception episode stays consistent, got %s", out.SessionConsistency)
	}
	if !out.RecheckRecommended {
		t.Fatal("guided => recheck recommended")
	}
	// wrap up B and close
	aaj(t, s, AskParams{Kind: KindTransfer, ConceptID: "c2", Text: "t?"}, Sure, alignedMech())
	mustFeedbackConcept(t, s, "c2")
	if err := s.Finalize("c2"); err != nil {
		t.Fatal(err)
	}
	if s.Phase != PhaseIntegrate {
		t.Fatalf("phase %s, want integrate", s.Phase)
	}
	aaj(t, s, AskParams{Kind: KindIntegrate, Text: "관계?"}, Sure, alignedMech())
	aaj(t, s, AskParams{Kind: KindIntegrate, Text: "관계 2?"}, Sure, alignedMech())
	rq := mustAsk(t, s, AskParams{Kind: KindReexplain, Text: "처음 질문 다시"})
	mustAnswer(t, s, Answer{QID: rq.QID, Action: AnswerText, Text: "재설명"})
	rep, err := s.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Concepts) != 2 || rep.Concepts[0].Phrase == "" {
		t.Fatal("report must carry learner phrasing for both concepts")
	}
	if s.Phase != PhaseDone {
		t.Fatal("session done")
	}
}

// ---- generative: transitions §2 predict rows -------------------------------

func TestGenerativePredictRow(t *testing.T) {
	type expect struct {
		pass      bool
		episode   bool
		recheck   bool
		investiga bool
	}
	cases := []struct {
		name string
		j    Judgment
		conf Confidence
		want expect
	}{
		{"aligned+rubric", Judgment{Alignment: Aligned, Support: SupMechanism, ModelClarity: ClarityExplicit}, Sure, expect{pass: true}},
		{"aligned no result", Judgment{Alignment: Aligned, Support: SupMechanism, ModelClarity: ClarityVague}, Sure, expect{}},
		{"contradicted sure", Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit}, Sure, expect{episode: true, recheck: true}},
		{"contradicted explicit unsure", Judgment{Alignment: Contradicted, ModelClarity: ClarityExplicit}, Unsure, expect{episode: true}},
		{"contradicted vague", Judgment{Alignment: Contradicted, ModelClarity: ClarityVague}, Unsure, expect{episode: true}},
		{"contradicted dontknow", Judgment{Alignment: Contradicted, ModelClarity: ClarityNone}, DontKnow, expect{episode: true}},
		{"partial", Judgment{Alignment: Partial, PartialCredit: "x"}, Unsure, expect{episode: true}},
		{"divergent", Judgment{Alignment: Divergent}, Sure, expect{investiga: true}},
		{"unknown", Judgment{Alignment: Unknown}, DontKnow, expect{episode: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newAISession(t, 1)
			aaj(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"}, tc.conf, tc.j)
			got := expect{
				pass:      s.Concept("c1").stage(StagePredict).State == StagePass,
				episode:   s.Episode != nil && s.Episode.Open,
				recheck:   s.NeedRecheck,
				investiga: s.openInvestigation() != nil,
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---- generative: transitions §2 why rows -----------------------------------

func TestGenerativeWhyRow(t *testing.T) {
	cases := []struct {
		name      string
		claimType ClaimType
		j         Judgment
		wantPass  bool
	}{
		{"mechanism", ClaimBehavior, Judgment{Alignment: Aligned, Support: SupMechanism}, true},
		{"evidence is not explanation", ClaimBehavior, Judgment{Alignment: Aligned, Support: SupEvidence}, false},
		{"convention non-norm", ClaimBehavior, Judgment{Alignment: Aligned, Support: SupConvention}, false},
		{"convention norm", ClaimNorm, Judgment{Alignment: Aligned, Support: SupConvention}, true},
		{"none", ClaimBehavior, Judgment{Alignment: Aligned, Support: SupNone}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := NewSession("t", SourceAI, RoleNone)
			c, _ := s.AddConcept(ConceptParams{Name: "a", ClaimText: "c", ClaimType: tc.claimType}, false)
			s.SetVerify(VerifyParams{ConceptID: c.ID, Status: VerifySupported,
				Evidence: []Evidence{{Kind: "execution", Text: "run"}}})
			s.Start()
			q := mustAsk(t, s, AskParams{Kind: KindOpen, Text: "open"})
			mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "…"})
			aaj(t, s, AskParams{Kind: KindPredict, ConceptID: c.ID, Text: "p?"}, Sure, alignedMech())
			aaj(t, s, AskParams{Kind: KindWhy, ConceptID: c.ID, Text: "w?"}, Sure, tc.j)
			got := s.Concept(c.ID).stage(StageWhy).State == StagePass
			if got != tc.wantPass {
				t.Fatalf("why pass = %v, want %v", got, tc.wantPass)
			}
			if !tc.wantPass {
				// one re-ask with changed format is allowed, then narrow
				if s.Concept(c.ID).stage(StageWhy).Reasks != 1 {
					t.Fatal("insufficient why gets exactly one format-changed re-ask")
				}
			}
		})
	}
}

// ---- explore / contested ----------------------------------------------------

func TestContestedExploreOpenOutcome(t *testing.T) {
	s, _ := NewSession("t", SourceConcept, RoleNone)
	c, _ := s.AddConcept(ConceptParams{Name: "a", ClaimText: "c"}, false)
	s.SetVerify(VerifyParams{ConceptID: c.ID, Status: VerifyContested,
		ExternalRefs: []string{"https://example.com/ref"}})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	q := mustAsk(t, s, AskParams{Kind: KindOpen, Text: "open"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerText, Text: "…"})
	// consequence is banned for contested
	_, err := s.Ask(AskParams{Kind: KindConsequence, ConceptID: c.ID, Text: "c?", EvidenceRef: "e1"})
	wantErr(t, err, "supported")
	// explore <= 2
	aaj(t, s, AskParams{Kind: KindExplore, ConceptID: c.ID, Text: "e1?"}, Unsure,
		Judgment{Alignment: Unknown})
	aaj(t, s, AskParams{Kind: KindExplore, ConceptID: c.ID, Text: "e2?"}, Unsure,
		Judgment{Alignment: Unknown})
	_, err = s.Ask(AskParams{Kind: KindExplore, ConceptID: c.ID, Text: "e3?"})
	wantErr(t, err, "capped")
	// open outcome requires explore feedback (P6)
	if err := s.Feedback(FeedbackConcept, c.ID, "wrap"); err != nil {
		t.Fatal(err)
	}
	err = s.Finalize(c.ID)
	wantErr(t, err, "explore feedback")
	if err := s.Feedback(FeedbackExplore, c.ID, "합의/갈림/필요 근거"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(c.ID); err != nil {
		t.Fatal(err)
	}
	out := s.Concept(c.ID).Outcome
	if out.LearningOutcome != OutcomeOpen || out.EndReason != EndUnresolved {
		t.Fatalf("contested explore => open/unresolved, got %s/%s", out.LearningOutcome, out.EndReason)
	}
}

// ---- objections -------------------------------------------------------------

func TestObjectionDiscardsWithoutJudgment(t *testing.T) {
	s := newAISession(t, 1)
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	code := mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerObject})
	if code != 7 {
		t.Fatalf("objection exit = %d, want 7", code)
	}
	if err := s.ResolveObjection(true, ""); err != nil {
		t.Fatal(err)
	}
	if s.JudgedCount != 0 {
		t.Fatal("objection: no judgment used")
	}
	// or investigation instead of discard
	q = mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p2?"})
	mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerObject})
	if err := s.ResolveObjection(false, "user contests the claim"); err != nil {
		t.Fatal(err)
	}
	if s.openInvestigation() == nil {
		t.Fatal("objection may open an investigation")
	}
}

// ---- I3/I4 response approval ------------------------------------------------

func TestAnswerQIDMustMatch(t *testing.T) {
	s := newAISession(t, 1)
	mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	_, err := s.AnswerQuestion(Answer{QID: "q999", Action: AnswerText, Text: "a"})
	wantErr(t, err, "I3")
}

func TestOneScreenOneQuestion(t *testing.T) {
	s := newAISession(t, 1)
	mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	_, err := s.Ask(AskParams{Kind: KindWhy, ConceptID: "c1", Text: "w?"})
	wantErr(t, err, "pending")
}

func TestAbortEndsSession(t *testing.T) {
	s := newAISession(t, 1)
	q := mustAsk(t, s, AskParams{Kind: KindPredict, ConceptID: "c1", Text: "p?"})
	code := mustAnswer(t, s, Answer{QID: q.QID, Action: AnswerAbort})
	if code != 4 || s.Phase != PhaseDone || !s.Aborted {
		t.Fatal("abort => exit 4, done, aborted")
	}
}
