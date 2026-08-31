package engine

// FeedbackKind gates (P6): recheck feedback unblocks the next ask, concept
// feedback unblocks finalize, explore feedback settles an open outcome.
type FeedbackKind string

const (
	FeedbackRecheck FeedbackKind = "recheck"
	FeedbackConcept FeedbackKind = "concept"
	FeedbackExplore FeedbackKind = "explore"
)

func (s *Session) Feedback(kind FeedbackKind, conceptID, text string) error {
	if len(s.PendingRestore) > 0 {
		return notAllowed("pending restore %v must be resolved first (I2)", s.PendingRestore)
	}
	if text == "" {
		return notAllowed("feedback requires content (P6 is about saying what was confirmed)")
	}
	switch kind {
	case FeedbackRecheck:
		if !s.NeedRecheckFeedback {
			return notAllowed("no recheck feedback due")
		}
		s.NeedRecheckFeedback = false
		return nil
	case FeedbackConcept:
		c := s.Concept(conceptID)
		if c == nil {
			return notAllowed("feedback --kind concept needs --concept")
		}
		c.FeedbackDone = true
		return nil
	case FeedbackExplore:
		c := s.Concept(conceptID)
		if c == nil {
			return notAllowed("feedback --kind explore needs --concept")
		}
		if c.ExploreCount == 0 {
			return notAllowed("no explore happened for %s", c.ID)
		}
		c.ExploreFeedbackDone = true
		return nil
	}
	return notAllowed("unknown feedback kind %q", kind)
}

// EpisodeClose ends the branch episode. Cause defaults to undetermined
// (P0-6); user_misconception requires the full checklist (P3).
func (s *Session) EpisodeClose(cause Cause, checklist *MisconceptionChecklist) error {
	if len(s.PendingRestore) > 0 {
		return notAllowed("pending restore %v must be resolved first (I2)", s.PendingRestore)
	}
	ep := s.Episode
	if ep == nil || !ep.Open {
		return notAllowed("no open episode")
	}
	if cause == "" {
		cause = CauseUndetermined
	}
	switch cause {
	case CauseUndetermined, CauseQuestionDefect, CauseAIError,
		CauseTerminology, CauseWeakCounterexample, CausePrerequisiteGap:
	case CauseUserMisconception:
		if checklist == nil || !checklist.AllTrue() {
			return notAllowed("user_misconception requires the full checklist to be true (P3)")
		}
	default:
		return notAllowed("unknown cause %q", cause)
	}
	ep.Cause = cause
	ep.Open = false
	c := s.Concept(ep.ConceptID)
	if c != nil {
		if cause == CauseUserMisconception {
			c.MisconceptionEpisodes++ // only this cause counts (P0-6)
		}
		if cause == CausePrerequisiteGap {
			c.Paused = true
		}
	}
	return nil
}

// ExplanationRecord: the escape hatch (or a normal user request, D31).
// After it, only own_words is allowed (P1).
func (s *Session) ExplanationRecord(conceptID, text string) error {
	if text == "" {
		return notAllowed("explanation text required")
	}
	c := s.Concept(conceptID)
	if c == nil {
		c = s.currentConceptObj()
	}
	if c == nil {
		return notAllowed("no concept to explain")
	}
	// Legitimate entries: user requested it (exit 6 / limit choice), the
	// misconception-x3 escape, retry/scaffold exhaustion, or recombine/hint
	// failure. Never while a recheck or restore is outstanding.
	if s.NeedRecheck || len(s.PendingRestore) > 0 {
		return notAllowed("resolve recheck/restore before explaining (I2/I6)")
	}
	allowed := s.explainRequested || c.MisconceptionEpisodes >= 3 || s.AwaitOwnWords
	if ep := s.Episode; ep != nil && ep.Open && ep.ConceptID == c.ID {
		if ep.RetryCount >= 2 || ep.ScaffoldCount >= 1 {
			allowed = true // scaffolding already failed at least once
		}
	}
	if !allowed {
		return notAllowed("explanation only via user request, the misconception-x3 escape, or after failed scaffolding (design §4.3)")
	}
	s.explainRequested = false
	c.ExplanationUsed = true
	if c.ExplanationStage == "" {
		if ep := s.Episode; ep != nil && ep.Open {
			c.ExplanationStage = ep.Stage
		}
	}
	if ep := s.Episode; ep != nil && ep.Open && ep.ConceptID == c.ID {
		ep.Open = false // explanation supersedes the episode
		if ep.Cause == "" {
			ep.Cause = CauseUndetermined
		}
	}
	s.ExplanationOpen = true
	s.AwaitOwnWords = true
	c.SupportEvents = append(c.SupportEvents, "explanation")
	return nil
}

// recheck is verify --recheck: re-examining the source of truth (I6, §4).
func (s *Session) recheck(c *Concept, p VerifyParams) error {
	if !s.NeedRecheck && s.openInvestigation() == nil {
		return notAllowed("no recheck or investigation outstanding")
	}
	for i := range p.Evidence {
		e := p.Evidence[i]
		e.ID = newID("e", &s.NextEID)
		e.Version = c.Claim.Version
		c.Evidence = append(c.Evidence, e)
	}
	if p.RecheckSupportsClaim {
		// claim confirmed against the source: feedback with the confirmed
		// result is mandatory before anything else (P6)
		s.NeedRecheck = false
		s.RecheckConcept = ""
		s.NeedRecheckFeedback = true
		if inv := s.openInvestigation(); inv != nil {
			inv.Open = false
			inv.Resolution = "supported"
		}
		return nil
	}
	// the claim lost: retraction is required next
	s.NeedRecheck = false
	s.RecheckConcept = ""
	s.retractDue = c.ID
	return nil
}

func (s *Session) advanceCurrent() {
	for _, c := range s.nonProvisionalConcepts() {
		if !c.Finalized && !c.Deferred {
			s.CurrentConcept = c.ID
			return
		}
	}
}

// maybeEnterIntegrate: transitions §0 — all concepts finalized/skipped.
// Entry needs one demonstrated concept AND a why pass on a DIFFERENT
// concept (D34, v4.1).
func (s *Session) maybeEnterIntegrate() {
	if s.Phase != PhaseLadder {
		return
	}
	for _, c := range s.nonProvisionalConcepts() {
		if !c.Finalized && !c.Deferred {
			return
		}
	}
	var demonstrated *Concept
	for _, c := range s.nonProvisionalConcepts() {
		if c.Outcome != nil && c.Outcome.LearningOutcome == OutcomeDemonstrated {
			demonstrated = c
			break
		}
	}
	if demonstrated != nil {
		for _, c := range s.nonProvisionalConcepts() {
			if c.ID != demonstrated.ID && c.stage(StageWhy).State == StagePass {
				s.Phase = PhaseIntegrate
				return
			}
		}
	}
	// otherwise: optional one-sentence relation question, then close
	s.Phase = PhaseClose
}
