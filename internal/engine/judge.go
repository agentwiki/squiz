package engine

// Judge applies the AI's structured judgment of the last answered question.
// This is the core of reduce(state, event): transitions §1, §2, §5.
func (s *Session) Judge(j Judgment) error {
	s.ensureRuntime()
	if len(s.PendingRestore) > 0 {
		return notAllowed("pending restore %v must be resolved first (I2)", s.PendingRestore)
	}
	if s.LastAnswer == nil || s.lastQuestion == nil {
		return notAllowed("no answer awaiting judgment")
	}
	if j.QID != s.LastAnswer.QID {
		return notAllowed("judge qid %q does not match answered %q (I3)", j.QID, s.LastAnswer.QID)
	}
	q := s.lastQuestion
	ans := s.LastAnswer
	if q.Kind == KindOpen || q.Kind == KindReexplain || q.Kind == KindSessionLimit {
		return notAllowed("%s answers are never judged (baseline only)", q.Kind)
	}
	if j.Confidence == "" {
		j.Confidence = ans.Confidence
	}
	c := s.Concept(q.ConceptID)
	if c == nil && s.Episode != nil {
		c = s.Concept(s.Episode.ConceptID)
	}
	if c == nil {
		c = s.currentConceptObj()
	}
	if c == nil {
		return notAllowed("cannot resolve concept for judgment")
	}
	// P7: partial requires explicit partial credit, at every stage.
	if j.Alignment == Partial && j.PartialCredit == "" {
		return notAllowed("partial judgment requires partial_credit (P7)")
	}
	// code --role author: why measures intent; the author is the authority.
	if s.Source == SourceCode && s.Role == RoleAuthor && q.Kind == KindWhy && j.Alignment == Contradicted {
		return notAllowed("author mode: why is intent and cannot be contradicted (design §3.5); use mismatch + awareness")
	}
	if j.ClaimVersion == 0 {
		j.ClaimVersion = c.Claim.Version
	}

	rec := JudgmentRecord{
		Judgment: j, ConceptID: c.ID, Kind: q.Kind, Stage: q.Stage,
		Scaffolded: s.Episode != nil && s.Episode.Open && s.Episode.Scaffolded && q.Kind != KindRetry,
	}
	s.Judgments = append(s.Judgments, rec)
	s.LastAnswer = nil
	s.lastQuestion = nil
	s.PostRecheckClarity = ""

	// I7: invalid questions — the answer is never used; judged not counted.
	if j.QuestionQuality != QValid {
		return s.handleInvalidQuestion(c, q, j)
	}
	if q.Kind != KindPrereq { // A5: recovery questions count in burden only
		s.JudgedCount++
	}

	switch q.Kind {
	case KindPredict, KindWhy, KindBoundary, KindTransfer:
		return s.judgeLadder(c, q, j)
	case KindRetry:
		return s.judgeRetry(c, q, j)
	case KindConsequence, KindNarrow, KindRecombine, KindModelClarify, KindHint:
		return s.judgeEpisode(c, q, j)
	case KindOwnWords:
		return s.judgeOwnWords(c, q, j)
	case KindIntegrate:
		s.IntegrateCount++
		if s.IntegrateCount >= 2 {
			s.Phase = PhaseClose
		}
		return nil
	case KindExplore:
		return nil // capped at ask time; feedback gate handles closure
	case KindAwareness:
		c.Incidents = append(c.Incidents, Incident{Type: "finding", Ref: q.QID})
		return nil
	case KindPrereq:
		if j.Alignment == Aligned {
			// prerequisite recovered: unpause downstream concepts
			for _, dc := range s.Concepts {
				dc.Paused = false
			}
		}
		return nil
	default:
		return notAllowed("kind %q is not judgeable", q.Kind)
	}
}

// handleInvalidQuestion implements transitions §5.
func (s *Session) handleInvalidQuestion(c *Concept, q *Question, j Judgment) error {
	s.recordDiscard(q, j.QuestionQuality)
	switch j.QuestionQuality {
	case QLeading:
		s.LeadingCount[c.ID]++
		if s.LeadingCount[c.ID] >= 2 {
			s.openInvestigationFor(c, "two leading questions on one concept: AI may be steering answers")
		}
	case QPrereqMissing:
		// prerequisite recovery (A5): pause downstream, episode cause if open
		c.Paused = true
		if s.Episode != nil && s.Episode.Open && s.Episode.ConceptID == c.ID {
			s.Episode.Cause = CausePrerequisiteGap
		}
	case QInaccessible:
		// rewritten with a different modality; nothing else to do
	}
	return nil
}

// stageRubricMet encodes design §3.3 per-stage rubrics onto the judgment
// fields the AI reports (P4 content stays an AI judgment; the mapping is
// the structural part the CLI can check).
func stageRubricMet(src Source, ct ClaimType, st Stage, j Judgment) bool {
	if j.Alignment != Aligned {
		return false
	}
	switch st {
	case StagePredict:
		return j.ModelClarity == ClarityExplicit
	case StageWhy:
		if j.Support == SupMechanism {
			return true
		}
		return j.Support == SupConvention && ct == ClaimNorm
	case StageBoundary, StageTransfer:
		return j.Support == SupMechanism
	}
	return false
}

func (s *Session) judgeLadder(c *Concept, q *Question, j Judgment) error {
	st := q.Stage
	if st == "" {
		st = ladderKinds[q.Kind]
	}
	si := c.stage(st)
	newCase := q.NewCase

	if newCase {
		c.NewCaseAttempted = true // A4/P1 gate: attempted, whatever the outcome
	}
	switch j.Alignment {
	case Aligned:
		if stageRubricMet(s.Source, c.Claim.Type, st, j) {
			path := PathSelfReached
			if newCase && c.ExplanationUsed {
				path = PathAfterExplanation
				c.NewCaseDone = true
			}
			s.passStage(c, st, path)
			// predict answers containing causality may waive why, but only
			// after a separate why-rubric judgment — the AI re-judges via a
			// why question; no shortcut here.
			return nil
		}
		// aligned but rubric unmet
		switch st {
		case StagePredict:
			// outcome not made explicit -> model_clarify (no episode yet)
			return nil // allowed: model_clarify (validated at ask)
		default:
			// why(evidence/convention-non-norm/none), boundary(result-only),
			// transfer(result-only): one re-ask with changed format, then narrow.
			if si.Reasks < 1 {
				si.Reasks++
				return nil // allowed: re-ask same stage with a new format
			}
			s.openEpisode(c, st)
			return nil // allowed: narrow (consequence banned by support kind)
		}
	case Contradicted:
		if st == StageTransfer {
			return s.transferSetback(c, st, j)
		}
		s.openEpisode(c, st)
		switch {
		case j.Confidence == Sure:
			s.NeedRecheck = true // I6
			s.RecheckConcept = c.ID
		case j.ModelClarity == ClarityExplicit:
			// allowed: consequence (verified evidence required at ask)
		case j.Confidence == DontKnow:
			// allowed: narrow / hint / explanation offer
		default:
			// vague/none -> model_clarify then narrow
		}
		return nil
	case Partial:
		if st == StageTransfer {
			return s.transferSetback(c, st, j)
		}
		s.openEpisode(c, st)
		return nil // allowed: narrow isolating the wrong part (credit recorded)
	case Divergent:
		s.openInvestigationFor(c, "user answer diverges from claim: suspect the claim first")
		if s.Source == SourceCode && s.Role == RoleAuthor && st == StagePredict {
			// author predict divergent: investigation/recheck first (D37)
			s.NeedRecheck = true
			s.RecheckConcept = c.ID
		}
		return nil
	case Unknown:
		if st == StageTransfer {
			return s.transferSetback(c, st, j)
		}
		s.openEpisode(c, st)
		return nil // allowed: narrow / hint choice / explanation offer
	case Mismatch:
		if !(s.Source == SourceCode && st == StageWhy) {
			return notAllowed("mismatch is only valid for code-mode why (intent)")
		}
		s.passStage(c, st, PathSelfReached) // why=pass with intent recorded
		return nil                          // allowed: neutral awareness question -> finding
	}
	return notAllowed("unknown alignment %q", j.Alignment)
}

// transferSetback: transitions §1 transfer rows (why return, then narrow).
func (s *Session) transferSetback(c *Concept, st Stage, j Judgment) error {
	if c.WhyReturns == 0 {
		c.WhyReturns = 1 // session_consistency=inconsistent via finalize
		// allowed: why re-confirmation then a fresh transfer case; the why
		// stage stays pass — this is re-confirmation, not regression.
		c.stage(StageTransfer).Reasks = 0
		return nil
	}
	s.openEpisode(c, st)
	if j.Alignment == Contradicted && j.Confidence == Sure {
		s.NeedRecheck = true
		s.RecheckConcept = c.ID
	}
	return nil // allowed: narrow once -> retry -> finalize
}

func (s *Session) passStage(c *Concept, st Stage, path AcquisitionPath) {
	si := c.stage(st)
	si.State = StagePass
	si.Path = path
	s.afterStagePass(c, st)
}

// afterStagePass drives transitions §0: concept ordering and the deferred
// transfer unlock (D35, A1).
func (s *Session) afterStagePass(c *Concept, st Stage) {
	if st == StageBoundary {
		concepts := s.nonProvisionalConcepts()
		idx := -1
		for i, cc := range concepts {
			if cc.ID == c.ID {
				idx = i
			}
		}
		// unlock the previous concept's transfer (A.transfer after B.boundary)
		if idx > 0 {
			prev := concepts[idx-1]
			if prev.stage(StageTransfer).State != StagePass {
				prev.TransferUnlocked = true
			}
		}
		if len(concepts) == 1 {
			// A1: single concept — transfer right before reexplain, no gap
			c.TransferUnlocked = true
		}
		if idx == len(concepts)-1 {
			// last concept: its own transfer unlocks right before integrate,
			// i.e. once every other concept's core ladder is settled
			c.TransferUnlocked = true
		}
		// move on to the next concept, if any (transfer of this one waits)
		if idx >= 0 && idx < len(concepts)-1 {
			s.CurrentConcept = concepts[idx+1].ID
		}
	}
}

func (s *Session) openEpisode(c *Concept, st Stage) {
	if s.Episode != nil && s.Episode.Open {
		return // already inside one; stays open until episode close
	}
	s.Episode = &Episode{
		ConceptID: c.ID, Stage: st, Cause: CauseUndetermined, Open: true,
	}
}

// judgeEpisode implements transitions §2 for scaffold-family questions.
func (s *Session) judgeEpisode(c *Concept, q *Question, j Judgment) error {
	ep := s.Episode
	if ep == nil || !ep.Open {
		// model_clarify can occur outside an episode (predict rubric miss)
		if q.Kind != KindModelClarify && q.Kind != KindConsequence {
			return notAllowed("%s judged outside an episode", q.Kind)
		}
	}
	switch q.Kind {
	case KindModelClarify:
		if j.ModelClarity == ClarityExplicit {
			s.PostRecheckClarity = ClarityExplicit
		}
		return nil // allowed: consequence (if verified) or narrow
	case KindConsequence:
		switch j.Alignment {
		case Aligned:
			// user recognized the contradiction -> unscaffolded retry with a
			// corrected model on a fresh scenario
			if ep != nil && ep.Open {
				ep.Await = KindRetry
			}
			return nil
		case Divergent:
			s.openInvestigationFor(c, "consequence answer diverges: suspect claim")
			return nil
		case Unknown:
			return nil // allowed: narrow
		default:
			// wrong consequence: present the executed result (feedback text
			// comes from the AI), then ask which assumption diverges
			return nil // allowed: consequence again or narrow
		}
	case KindNarrow:
		switch {
		case j.Alignment == Aligned:
			if ep != nil {
				ep.Await = KindRecombine // P1: mandatory
			}
			return nil
		case j.Alignment == Partial:
			return nil // narrow again within the limit
		case j.Alignment == Contradicted && j.Confidence == Sure:
			s.NeedRecheck = true // I6
			s.RecheckConcept = c.ID
			return nil
		default:
			return nil // allowed: hint (user-chosen level)
		}
	case KindRecombine:
		if j.Alignment == Aligned {
			if ep != nil {
				ep.Await = KindRetry // P1: mandatory unscaffolded retry
			}
			return nil
		}
		return nil // allowed: hint or explanation offer
	case KindHint:
		if j.Alignment == Aligned {
			if ep != nil {
				ep.Await = KindRetry
			}
			return nil
		}
		return nil // allowed: next hint level (user choice) or explanation
	}
	return notAllowed("unhandled episode kind %q", q.Kind)
}

func (s *Session) judgeRetry(c *Concept, q *Question, j Judgment) error {
	ep := s.Episode
	if ep == nil || !ep.Open {
		return notAllowed("retry outside an episode")
	}
	if stageRubricMet(s.Source, c.Claim.Type, ep.Stage, j) {
		// unscaffolded pass: stage final (P1), path=guided
		s.passStage(c, ep.Stage, PathGuided)
		ep.Await = ""
		ep.ReadyToClose = true
		return nil // allowed: episode close, then the ladder continues
	}
	ep.RetryCount++
	if ep.RetryCount >= 2 {
		// A2: retry cap — explanation or skip only
		return nil
	}
	return nil // allowed: re-scaffold within limits, or explanation
}

func (s *Session) judgeOwnWords(c *Concept, q *Question, j Judgment) error {
	if j.Alignment == Aligned && j.Support == SupMechanism {
		// genuine reconstruction -> new-case boundary or transfer only (P1)
		c.OwnWordsPassed = true
		s.AwaitOwnWords = false
		s.AwaitNewCase = true
		return nil
	}
	if j.Alignment == Aligned && !s.OwnWordsRepeatUsed {
		// repetition of the explanation: one format-changed application task
		s.OwnWordsRepeatUsed = true
		return nil // allowed: own_words once more (application format)
	}
	// insufficient: further explanation or skip
	s.AwaitOwnWords = true
	s.ExplanationOpen = false
	return nil
}

func (s *Session) openInvestigationFor(c *Concept, reason string) *Investigation {
	inv := Investigation{
		ID: newID("inv", &s.NextEID), ConceptID: c.ID, Reason: reason, Open: true,
	}
	s.Investigations = append(s.Investigations, inv)
	return &s.Investigations[len(s.Investigations)-1]
}

func newID(prefix string, ctr *int) string {
	id := *ctr
	*ctr = id + 1
	return prefix + itoa(id)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
