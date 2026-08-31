package engine

// computeOutcome implements transitions §8 / design §3.4. The AI never sets
// these fields; they are derived (P2, P5).
func (s *Session) computeOutcome(c *Concept) *Outcome {
	passes := 0
	anyGuided, anyAfterExpl := false, false
	for _, st := range Ladder {
		si := c.stage(st)
		if si.State == StagePass {
			passes++
			switch si.Path {
			case PathGuided:
				anyGuided = true
			case PathAfterExplanation:
				anyAfterExpl = true
			}
		}
	}
	out := &Outcome{
		Retention:     "untested", // P2: never updatable in-session
		SupportEvents: append([]string(nil), c.SupportEvents...),
		Incidents:     append([]Incident(nil), c.Incidents...),
		Vindicated:    append([]string(nil), c.Vindicated...),
	}
	switch {
	case passes == 4:
		out.LearningOutcome = OutcomeDemonstrated
	case passes >= 2:
		out.LearningOutcome = OutcomePartial
	default:
		out.LearningOutcome = OutcomeNotDemonstrated
	}
	if c.Verify != VerifySupported && c.ExploreCount > 0 {
		out.LearningOutcome = OutcomeOpen
		out.EndReason = EndUnresolved
	}
	switch {
	case c.AbortedMid:
		out.AcquisitionPath = PathStopped
	case c.ExplanationUsed || anyAfterExpl:
		out.AcquisitionPath = PathAfterExplanation
	case anyGuided || len(c.SupportEvents) > 0:
		out.AcquisitionPath = PathGuided
	default:
		out.AcquisitionPath = PathSelfReached
	}
	inconsistent := c.MisconceptionEpisodes >= 2 || c.WhyReturns > 0 ||
		c.TaintContrib || (s.Phase == PhaseDone && !s.ReexplainDone && !s.Aborted)
	if inconsistent {
		out.SessionConsistency = "inconsistent"
	} else {
		out.SessionConsistency = "consistent"
	}
	lastUnsure := false
	for i := len(s.Judgments) - 1; i >= 0; i-- {
		if s.Judgments[i].ConceptID == c.ID && !s.Judgments[i].Tainted {
			lastUnsure = s.Judgments[i].Confidence == Unsure
			break
		}
	}
	out.RecheckRecommended = inconsistent ||
		out.AcquisitionPath == PathGuided || out.AcquisitionPath == PathAfterExplanation ||
		lastUnsure
	if out.EndReason == "" {
		switch {
		case s.Aborted:
			out.EndReason = EndAborted
		case s.JudgedCount >= 30 && out.LearningOutcome != OutcomeDemonstrated:
			out.EndReason = EndLimit
		default:
			out.EndReason = EndCompleted
		}
	}
	if len(s.nonProvisionalConcepts()) == 1 {
		out.TransferGapNote = true // A1
	}
	return out
}

// Finalize closes one concept: transitions §0/§3 gates + §8 computation.
func (s *Session) Finalize(conceptID string) error {
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	if c.Finalized {
		return notAllowed("concept %s already finalized", c.ID)
	}
	if ep := s.Episode; ep != nil && ep.Open && ep.ConceptID == c.ID {
		return notAllowed("close the open episode first")
	}
	if len(s.PendingRestore) > 0 {
		return notAllowed("pending restore blocks finalize (I2)")
	}
	if s.NeedRecheckFeedback {
		return notAllowed("recheck feedback due before finalize (P6)")
	}
	if !c.FeedbackDone {
		return notAllowed("feedback --kind concept required before finalize (P6)")
	}
	if c.Verify != VerifySupported && c.ExploreCount > 0 && !c.ExploreFeedbackDone {
		return notAllowed("explore feedback (agreed/diverging/needed evidence) required before an open outcome (P6)")
	}
	// P1: explanation without an independent new-case attempt cannot
	// finalize — unless the burden limit forces an early close (A4).
	if c.ExplanationUsed && !c.NewCaseAttempted && !s.LimitChoiceHandled && !s.Aborted {
		return notAllowed("explanation used: a new-case boundary/transfer must be attempted before finalize (P1/A4)")
	}
	c.Outcome = s.computeOutcome(c)
	c.Finalized = true
	s.advanceCurrent()
	s.maybeEnterIntegrate()
	return nil
}

// SkipConcept is the CLI-side skip (exit 8 does the same via the TUI).
func (s *Session) SkipConcept(conceptID string) error {
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	if c.Finalized {
		return notAllowed("concept %s already finalized", c.ID)
	}
	s.deferConcept(c)
	return nil
}

// Report is the close output (design §4.5 required items).
type Report struct {
	SessionID           string          `json:"session_id"`
	Source              Source          `json:"source"`
	OpenBaseline        string          `json:"open_baseline"`
	ReexplainText       string          `json:"reexplain_text"`
	Concepts            []ConceptReport `json:"concepts"`
	JudgedCount         int             `json:"judged_count"`
	BurdenCount         int             `json:"burden_count"`
	SharedBlindSpotNote string          `json:"shared_blind_spot_note"`
	RecheckAdvice       string          `json:"recheck_advice"`
}

type ConceptReport struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Outcome *Outcome `json:"outcome"`
	// Learner-facing phrasing (design §3.4): no internal jargon.
	Phrase string `json:"phrase"`
}

func learnerPhrase(o *Outcome) string {
	if o == nil {
		return "이번 대화에서는 아직 확인되지 않음"
	}
	switch o.LearningOutcome {
	case OutcomeDemonstrated:
		return "이 세션에서 독립적으로 설명·적용함"
	case OutcomePartial:
		if o.AcquisitionPath == PathAfterExplanation {
			return "설명 후 새 사례에 적용함"
		}
		return "단서를 활용해 재구성함"
	case OutcomeOpen:
		return "합의·미결 지점을 정리하고 열린 상태로 종료함"
	case OutcomeDeferred:
		return "다음 기회로 미룸"
	default:
		return "이번 대화에서는 아직 확인되지 않음"
	}
}

// Close ends the session. Requires every concept settled; reexplain must
// have run unless the session was aborted or summary-closed (P8 choice).
func (s *Session) Close() (*Report, error) {
	if s.Phase == PhaseDone {
		return nil, notAllowed("already done")
	}
	for _, c := range s.nonProvisionalConcepts() {
		if !c.Finalized && !c.Deferred {
			if !s.summaryClose && !s.Aborted {
				return nil, notAllowed("concept %s not finalized/skipped", c.ID)
			}
			c.Outcome = s.computeOutcome(c)
			if s.summaryClose {
				c.Outcome.EndReason = EndLimit
			}
			c.Finalized = true
		}
	}
	if s.Phase == PhaseClose && !s.ReexplainDone && !s.summaryClose && !s.Aborted {
		return nil, notAllowed("reexplain before close (missing it marks inconsistency)")
	}
	if !s.ReexplainDone && !s.Aborted {
		// reexplain skipped (summary close / integrate shortcut): the
		// session cannot vouch for consistency (transitions §8)
		for _, c := range s.nonProvisionalConcepts() {
			if c.Outcome != nil && c.Outcome.LearningOutcome != OutcomeDeferred {
				c.Outcome.SessionConsistency = "inconsistent"
				c.Outcome.RecheckRecommended = true
			}
		}
	}
	s.Phase = PhaseDone
	rep := &Report{
		SessionID: s.ID, Source: s.Source,
		OpenBaseline: s.OpenBaseline, ReexplainText: s.reexplainText,
		JudgedCount: s.JudgedCount, BurdenCount: s.BurdenCount,
		RecheckAdvice: "다른 날 짧은 재확인 권장 (retention은 이 세션에서 판정 불가, P2)",
	}
	if s.Source == SourceConcept {
		rep.SharedBlindSpotNote = "사용자 답과 AI 가설이 일치한 지점은 이 세션이 의심하지 못했다 (design §3.2)"
	} else {
		rep.SharedBlindSpotNote = "실행으로 확인되지 않은 일치 지점은 공유된 맹점일 수 있다 (design §3.2)"
	}
	for _, c := range s.nonProvisionalConcepts() {
		rep.Concepts = append(rep.Concepts, ConceptReport{
			ID: c.ID, Name: c.Name, Outcome: c.Outcome, Phrase: learnerPhrase(c.Outcome),
		})
	}
	return rep, nil
}
