package engine

// Retract withdraws the AI's claim (or a piece of evidence). Version bumps,
// dependent judgments are tainted (I5) and queued for restore (I2).
func (s *Session) Retract(conceptID, kind, ref string) error {
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	if len(s.PendingRestore) > 0 {
		return notAllowed("pending restore %v must be resolved first (I2)", s.PendingRestore)
	}
	if s.retractDue != "" && s.retractDue != c.ID {
		return notAllowed("recheck refuted the claim for %s: retract that one first", s.retractDue)
	}
	switch kind {
	case "claim", "":
		c.Claim.Version++
		c.Incidents = append(c.Incidents, Incident{Type: "claim_retracted", Ref: c.ID})
	case "evidence":
		e := c.evidence(ref)
		if e == nil {
			return notAllowed("unknown evidence %q", ref)
		}
		e.Retracted = true
		c.Incidents = append(c.Incidents, Incident{Type: "evidence_retracted", Ref: ref})
	default:
		return notAllowed("retract kind must be claim or evidence")
	}
	s.taintDependents(c, kind, ref)
	// only clear obligations that pointed at THIS concept
	if s.retractDue == c.ID {
		s.retractDue = ""
	}
	if s.RecheckConcept == c.ID {
		s.NeedRecheck = false
		s.RecheckConcept = ""
	}
	if inv := s.openInvestigation(); inv != nil && inv.ConceptID == c.ID {
		inv.Open = false
		inv.Resolution = "retracted"
	}
	return nil
}

func (s *Session) taintDependents(c *Concept, kind, ref string) {
	for i := range s.Judgments {
		j := &s.Judgments[i]
		// Restored records are NOT exempt: a re-judgment made against a
		// version that is later retracted must be tainted again (I5).
		if j.ConceptID != c.ID || j.Tainted {
			continue
		}
		hit := false
		if kind == "claim" || kind == "" {
			hit = j.ClaimVersion < c.Claim.Version
		} else {
			for _, er := range j.EvidenceRefs {
				if er == ref {
					hit = true
				}
			}
		}
		if hit {
			j.Tainted = true
			c.TaintContrib = true
			s.PendingRestore = append(s.PendingRestore, j.QID)
		}
	}
	if len(s.PendingRestore) > 0 {
		s.recomputeStages(c)
	}
}

// Restore re-judges one tainted answer against the new claim version (I2).
func (s *Session) Restore(j Judgment) error {
	if len(s.PendingRestore) == 0 {
		return notAllowed("nothing pending restore")
	}
	idx := -1
	for i, qid := range s.PendingRestore {
		if qid == j.QID {
			idx = i
		}
	}
	if idx < 0 {
		return notAllowed("qid %q is not pending restore (%v)", j.QID, s.PendingRestore)
	}
	var orig *JudgmentRecord
	for i := range s.Judgments {
		if s.Judgments[i].QID == j.QID && s.Judgments[i].Tainted {
			orig = &s.Judgments[i]
		}
	}
	if orig == nil {
		return notAllowed("no tainted judgment for %q", j.QID)
	}
	c := s.Concept(orig.ConceptID)
	if j.Alignment == Partial && j.PartialCredit == "" {
		return notAllowed("partial judgment requires partial_credit (P7)")
	}
	rec := JudgmentRecord{
		Judgment: j, ConceptID: orig.ConceptID, Kind: orig.Kind,
		Stage: orig.Stage, Scaffolded: orig.Scaffolded, Restored: true,
	}
	rec.ClaimVersion = c.Claim.Version
	s.Judgments = append(s.Judgments, rec)
	s.PendingRestore = append(s.PendingRestore[:idx], s.PendingRestore[idx+1:]...)
	// recompute this judgment's concept immediately — the queue may span
	// several concepts and each must be re-derived from its own restores
	s.recomputeStages(c)
	return nil
}

// recomputeStages re-derives ladder stage states from the untainted
// judgment history (design §11: retroactive re-judgment).
func (s *Session) recomputeStages(c *Concept) {
	for _, st := range Ladder {
		si := c.stage(st)
		si.State = StagePending
		prevPath := si.Path
		si.Path = ""
		for _, j := range s.Judgments {
			if j.ConceptID != c.ID || j.Tainted || j.QuestionQuality != QValid {
				continue
			}
			target := j.Stage
			if target == "" {
				target = ladderKinds[j.Kind]
			}
			if target != st {
				continue
			}
			if !stageRubricMet(s.Source, c.Claim.Type, st, j.Judgment) {
				continue
			}
			if j.Kind == KindRetry {
				si.State = StagePass
				si.Path = PathGuided
			} else if !j.Scaffolded {
				si.State = StagePass
				if si.Path == "" {
					si.Path = PathSelfReached
					if prevPath == PathAfterExplanation {
						si.Path = PathAfterExplanation
					}
				}
			} else {
				si.State = StageScaffolded
			}
		}
	}
}

// Defect (ai mode): the user's challenge exposed a real defect. Only the
// executed, confirmed propositions are vindicated — no stage or concept
// passes: finding a defect is not the same as understanding the concept.
func (s *Session) Defect(conceptID, description string, vindicated []string) error {
	if s.Source != SourceAI {
		return notAllowed("defect is an ai-mode event; use finding in code mode")
	}
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	if len(vindicated) == 0 {
		return notAllowed("defect requires vindicated_propositions")
	}
	c.Incidents = append(c.Incidents, Incident{Type: "defect_found", Ref: description})
	c.Vindicated = append(c.Vindicated, vindicated...)
	c.Claim.Version++ // the claim was wrong; co-fix follows
	s.taintDependents(c, "claim", "")
	if inv := s.openInvestigation(); inv != nil && inv.ConceptID == c.ID {
		inv.Open = false
		inv.Resolution = "retracted"
	}
	s.NeedRecheck = false
	return nil
}

// Finding (code mode): behavior != intent, recorded without fixing.
func (s *Session) Finding(conceptID, ref string) error {
	if s.Source != SourceCode {
		return notAllowed("finding is a code-mode event")
	}
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	c.Incidents = append(c.Incidents, Incident{Type: "finding", Ref: ref})
	return nil
}

// InvestigationOpen: explicit claim-level doubt (user objection, contested
// contradicted+sure, ...). Locks consequence (I1).
func (s *Session) InvestigationOpen(conceptID, reason string) error {
	c := s.Concept(conceptID)
	if c == nil {
		return notAllowed("unknown concept %q", conceptID)
	}
	if s.openInvestigation() != nil {
		return notAllowed("an investigation is already open")
	}
	s.openInvestigationFor(c, reason)
	s.objectionPending = nil
	return nil
}

// InvestigationClose with resolution unresolved -> explore path (contested).
func (s *Session) InvestigationClose(resolution string) error {
	inv := s.openInvestigation()
	if inv == nil {
		return notAllowed("no open investigation")
	}
	switch resolution {
	case "supported", "retracted", "unresolved":
	default:
		return notAllowed("resolution must be supported|retracted|unresolved")
	}
	if resolution == "supported" {
		// confirmed against source: feedback gate applies (P6)
		s.NeedRecheckFeedback = true
	}
	inv.Open = false
	inv.Resolution = resolution
	return nil
}

// ResolveObjection: exit 7 — re-evaluate question quality or investigate.
// discard=true drops the question with no judgment (I7).
func (s *Session) ResolveObjection(discard bool, reason string) error {
	if s.objectionPending == nil {
		return notAllowed("no objection outstanding")
	}
	q := s.objectionPending
	s.objectionPending = nil
	if discard {
		s.recordDiscard(q, QAmbiguous)
		return nil
	}
	c := s.Concept(q.ConceptID)
	if c == nil {
		c = s.currentConceptObj()
	}
	if c == nil {
		return notAllowed("no concept for objection investigation")
	}
	if reason == "" {
		reason = "user objection to " + q.QID
	}
	s.openInvestigationFor(c, reason)
	return nil
}
