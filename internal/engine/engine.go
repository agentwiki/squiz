package engine

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotAllowed wraps every I8 rejection so callers can distinguish rule
// violations from I/O errors.
var ErrNotAllowed = errors.New("not allowed in current state")

func notAllowed(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrNotAllowed, fmt.Sprintf(format, a...))
}

func NewSession(id string, source Source, role Role) (*Session, error) {
	switch source {
	case SourceAI, SourceConcept, SourceCode:
	default:
		return nil, notAllowed("unknown source %q", source)
	}
	if role != RoleNone && source != SourceCode {
		return nil, notAllowed("--role only applies to --source code")
	}
	return &Session{
		ID: id, Source: source, Role: role, Phase: PhasePrep,
		NextQID: 1, NextEID: 1,
		LeadingCount: map[string]int{},
	}, nil
}

type ConceptParams struct {
	Name                   string    `json:"name"`
	ClaimText              string    `json:"claim"`
	ClaimType              ClaimType `json:"claim_type"`
	TargetPerformance      string    `json:"target_performance"`
	Depth                  string    `json:"depth"`
	AcceptedAlternatives   []string  `json:"accepted_alternatives"`
	Prerequisites          []string  `json:"prerequisites"`
	ExpectedMisconceptions []string  `json:"expected_misconceptions"`
}

// AddConcept registers a concept during prep. During ladder it is only
// allowed as a provisional prerequisite registration.
func (s *Session) AddConcept(p ConceptParams, provisional bool) (*Concept, error) {
	if p.Name == "" || p.ClaimText == "" {
		return nil, notAllowed("concept needs name and claim")
	}
	if provisional {
		if s.Phase != PhaseLadder {
			return nil, notAllowed("provisional (prerequisite) concepts only during ladder")
		}
		if s.PrereqDepth >= 1 {
			return nil, notAllowed("prerequisite recovery depth cap is 1; defer the downstream concept instead")
		}
		s.PrereqDepth++
	} else {
		if s.Phase != PhasePrep {
			return nil, notAllowed("concept add only during prep")
		}
		if len(s.nonProvisionalConcepts()) >= 5 {
			return nil, notAllowed("max 5 concepts (design §5.1)")
		}
	}
	ct := p.ClaimType
	if ct == "" {
		ct = ClaimBehavior
	}
	switch ct {
	case ClaimBehavior, ClaimIntent, ClaimFact, ClaimNorm:
	default:
		return nil, notAllowed("unknown claim_type %q", ct)
	}
	c := &Concept{
		ID:                fmt.Sprintf("c%d", len(s.Concepts)+1),
		Name:              p.Name,
		Claim:             Claim{Text: p.ClaimText, Type: ct, Version: 1},
		Verify:            VerifyPending,
		TargetPerformance: p.TargetPerformance, Depth: p.Depth,
		AcceptedAlternatives:   p.AcceptedAlternatives,
		Prerequisites:          p.Prerequisites,
		ExpectedMisconceptions: p.ExpectedMisconceptions,
		Stages:                 map[Stage]*StageInfo{},
		Provisional:            provisional,
	}
	for _, st := range Ladder {
		c.Stages[st] = &StageInfo{State: StagePending}
	}
	s.Concepts = append(s.Concepts, c)
	return c, nil
}

func (s *Session) nonProvisionalConcepts() []*Concept {
	var out []*Concept
	for _, c := range s.Concepts {
		if !c.Provisional {
			out = append(out, c)
		}
	}
	return out
}

type VerifyParams struct {
	ConceptID            string       `json:"concept_id"`
	Status               VerifyStatus `json:"status"`
	ExternalRefs         []string     `json:"external_refs"`
	Evidence             []Evidence   `json:"evidence"`
	Recheck              bool         `json:"recheck"`
	RecheckSupportsClaim bool         `json:"recheck_supports_claim"`
}

// SetVerify records verification. With Recheck it is the I6 recheck of a
// standing claim against the source of truth.
func (s *Session) SetVerify(p VerifyParams) error {
	c := s.Concept(p.ConceptID)
	if c == nil {
		return notAllowed("unknown concept %q", p.ConceptID)
	}
	if p.Recheck {
		return s.recheck(c, p)
	}
	if s.Phase != PhasePrep {
		return notAllowed("verify (non-recheck) only during prep")
	}
	switch p.Status {
	case VerifySupported, VerifyRefuted, VerifyUnverifiable:
	case VerifyContested:
		if s.Source != SourceConcept {
			return notAllowed("contested only in concept mode")
		}
	case VerifyIntentGuess:
		if s.Source != SourceCode {
			return notAllowed("intent_guess only in code mode")
		}
	default:
		return notAllowed("unknown verify status %q", p.Status)
	}
	if p.Status == VerifySupported {
		if s.Source == SourceConcept && len(p.ExternalRefs) == 0 {
			return notAllowed("concept mode: supported requires external refs (design §4)")
		}
		if s.Source != SourceConcept && len(p.Evidence) == 0 {
			return notAllowed("%s mode: supported requires executed counterexample evidence", s.Source)
		}
	}
	c.Verify = p.Status
	c.ExternalRefs = p.ExternalRefs
	for i := range p.Evidence {
		e := p.Evidence[i]
		e.ID = fmt.Sprintf("e%d", s.NextEID)
		s.NextEID++
		e.Version = c.Claim.Version
		c.Evidence = append(c.Evidence, e)
	}
	return nil
}

// Start moves prep -> open (transitions §1 row 1).
func (s *Session) Start() error {
	if s.Phase != PhasePrep {
		return notAllowed("start only from prep")
	}
	cs := s.nonProvisionalConcepts()
	if len(cs) == 0 {
		return notAllowed("no concepts registered")
	}
	for _, c := range cs {
		if c.Verify == VerifyPending || c.Verify == VerifyRefuted {
			return notAllowed("concept %s verify=%s blocks start", c.ID, c.Verify)
		}
	}
	s.Phase = PhaseOpen
	s.CurrentConcept = cs[0].ID
	return nil
}

type AskParams struct {
	Kind        QuestionKind `json:"kind"`
	ConceptID   string       `json:"concept_id"`
	Stage       Stage        `json:"stage"`
	Text        string       `json:"text"`
	HintLevel   int          `json:"hint_level"`
	Options     []string     `json:"options"`
	EvidenceRef string       `json:"evidence_ref"`
	NewCase     bool         `json:"new_case"`
}

// Ask registers a question (one screen, one question). Every ask is a
// response-requiring screen, so it counts toward burden (P8).
func (s *Session) Ask(p AskParams) (*Question, error) {
	if s.Pending != nil {
		return nil, notAllowed("question %s is pending; one screen one question", s.Pending.QID)
	}
	if s.LastAnswer != nil {
		return nil, notAllowed("answer %s awaits judge", s.LastAnswer.QID)
	}
	if s.Phase == PhaseDone {
		return nil, notAllowed("session is done")
	}
	if len(s.PendingRestore) > 0 {
		return nil, notAllowed("pending restore %v must be resolved first (I2)", s.PendingRestore)
	}
	if s.NeedRecheck {
		return nil, notAllowed("recheck required before further questions (I6): run `squiz verify --recheck`")
	}
	if s.NeedRecheckFeedback {
		return nil, notAllowed("feedback --kind recheck required before next ask (P6)")
	}
	if s.retractDue != "" {
		return nil, notAllowed("recheck refuted the claim for %s: retract it first", s.retractDue)
	}
	if s.LimitChoicePending && p.Kind != KindSessionLimit {
		return nil, notAllowed("burden limit reached: present the continue/summary/explanation choice first (P8)")
	}
	if p.Text == "" {
		return nil, notAllowed("question text required")
	}
	if err := s.validateAskKind(&p); err != nil {
		return nil, err
	}

	q := &Question{
		QID: fmt.Sprintf("q%d", s.NextQID), Kind: p.Kind,
		ConceptID: p.ConceptID, Stage: p.Stage, Text: p.Text,
		HintLevel: p.HintLevel, NewCase: p.NewCase,
		Options: p.Options, EvidenceRef: p.EvidenceRef,
	}
	s.NextQID++
	s.Pending = q
	s.BurdenCount++ // every response-requiring screen counts toward burden
	s.noteAskSideEffects(&p)
	// P8 burden gate. The ratio check only kicks in past 10 screens so
	// early-session noise (burden 2 vs judged 1) does not trip it.
	if !s.LimitChoiceHandled && !s.LimitChoicePending &&
		(s.BurdenCount > 30 || (s.BurdenCount >= 10 && float64(s.BurdenCount) > 1.5*float64(s.JudgedCount))) {
		s.LimitChoicePending = true
	}
	return q, nil
}

// learningKinds are new learning questions, blocked at judged >= 30.
var learningKinds = map[QuestionKind]bool{
	KindPredict: true, KindWhy: true, KindBoundary: true, KindTransfer: true,
	KindConsequence: true, KindNarrow: true, KindRecombine: true, KindRetry: true,
	KindModelClarify: true, KindHint: true, KindExplore: true, KindOwnWords: true,
	KindIntegrate: true,
}

func (s *Session) validateAskKind(p *AskParams) error {
	k := p.Kind
	if s.JudgedCount >= 30 && learningKinds[k] {
		return notAllowed("judged count %d >= 30: new learning questions blocked (P8); wrap up or close", s.JudgedCount)
	}
	c := s.Concept(p.ConceptID)

	// Explanation chain gates (P1): once explanation is recorded, only
	// own_words; after own_words, only a new-case boundary/transfer.
	if s.AwaitOwnWords && k != KindOwnWords {
		return notAllowed("after explanation only own_words is allowed (P1)")
	}
	if s.AwaitNewCase && !(k == KindBoundary || k == KindTransfer) {
		return notAllowed("after own_words only a new-case boundary or transfer is allowed (P1)")
	}
	if s.AwaitNewCase && !p.NewCase {
		return notAllowed("post-explanation %s must be a new case (P1): pass --new-case", k)
	}
	if s.ExplanationOpen && k != KindOwnWords {
		return notAllowed("explanation recorded; own_words next (P1)")
	}

	// Episode fading chain (P1): Await pins the mandatory next kind.
	if ep := s.Episode; ep != nil && ep.Open && ep.Await != "" {
		switch ep.Await {
		case KindRecombine:
			if k != KindRecombine {
				return notAllowed("narrow aligned requires recombine next (P1)")
			}
		case KindRetry:
			if k != KindRetry {
				return notAllowed("unscaffolded retry required next (P1)")
			}
		}
	}

	switch k {
	case KindOpen:
		if s.Phase != PhaseOpen {
			return notAllowed("open question only in open phase")
		}
	case KindReexplain:
		if s.Phase != PhaseClose {
			return notAllowed("reexplain only in close phase")
		}
	case KindIntegrate:
		if s.Phase != PhaseIntegrate {
			return notAllowed("integrate questions only in integrate phase")
		}
		if s.IntegrateCount >= 2 {
			return notAllowed("integrate judgments capped at 2")
		}
	case KindSessionLimit:
		if !s.LimitChoicePending {
			return notAllowed("no session limit pending")
		}
		if len(p.Options) < 3 {
			return notAllowed("session_limit_choice needs continue/summary/explanation options")
		}
	case KindPredict, KindWhy, KindBoundary, KindTransfer:
		if s.Phase != PhaseLadder {
			return notAllowed("ladder questions only in ladder phase")
		}
		if c == nil {
			return notAllowed("ladder question needs --concept")
		}
		if c.Paused {
			return notAllowed("concept %s paused for prerequisite recovery", c.ID)
		}
		if c.Finalized || c.Deferred {
			return notAllowed("concept %s already finalized/deferred", c.ID)
		}
		if ep := s.Episode; ep != nil && ep.Open && !(s.AwaitNewCase && p.NewCase) {
			return notAllowed("episode open for %s/%s: close it before new ladder questions", ep.ConceptID, ep.Stage)
		}
		st := ladderKinds[k]
		if p.Stage != "" && p.Stage != st {
			return notAllowed("--stage %s conflicts with --kind %s", p.Stage, k)
		}
		p.Stage = st
		if st == StageTransfer && !c.TransferUnlocked && !(s.AwaitNewCase && p.NewCase) {
			return notAllowed("transfer for %s is deferred until the next concept's boundary", c.ID)
		}
		if c.stage(st).State == StagePass && !(s.AwaitNewCase && p.NewCase) {
			// exception: why re-confirmation after a transfer setback
			// (transitions §2: transfer contradicted, why_returns=0)
			whyReturn := st == StageWhy && c.WhyReturns > 0 &&
				c.stage(StageTransfer).State != StagePass
			if !whyReturn {
				return notAllowed("stage %s already passed for %s", st, c.ID)
			}
		}
	case KindConsequence:
		// I1: explicit model + verified excluding evidence + no investigation.
		if c == nil {
			return notAllowed("consequence needs --concept")
		}
		if s.openInvestigation() != nil {
			return notAllowed("consequence locked while an investigation is open (I1)")
		}
		if s.NeedRecheck {
			return notAllowed("consequence locked pending recheck (I6)")
		}
		if c.Verify != VerifySupported {
			return notAllowed("consequence requires verify=supported (contested/unverifiable ban it)")
		}
		if !s.lastClarityExplicit(c.ID) {
			return notAllowed("consequence requires model_clarity=explicit_prediction on the user's model")
		}
		if p.EvidenceRef == "" || c.evidence(p.EvidenceRef) == nil || c.evidence(p.EvidenceRef).Retracted {
			return notAllowed("consequence requires --evidence <id> pointing at verified, unretracted evidence (I1)")
		}
	case KindNarrow, KindHint:
		ep := s.Episode
		if ep == nil || !ep.Open {
			return notAllowed("%s only inside an episode", k)
		}
		if ep.ScaffoldCount >= 3 {
			return notAllowed("scaffold limit (3) reached: explanation or skip only")
		}
		if k == KindHint && (p.HintLevel < 1 || p.HintLevel > 4) {
			return notAllowed("hint needs --level 1..4")
		}
	case KindModelClarify:
		// diagnostic, allowed inside or right after a ladder judgment
	case KindRecombine:
		ep := s.Episode
		if ep == nil || !ep.Open || ep.Await != KindRecombine {
			return notAllowed("recombine only after narrow aligned (P1)")
		}
	case KindRetry:
		ep := s.Episode
		if ep == nil || !ep.Open {
			return notAllowed("retry only inside an episode")
		}
		if ep.Await != KindRetry && ep.Await != "" {
			return notAllowed("fading chain requires %s next", ep.Await)
		}
		if ep.RetryCount >= 2 {
			return notAllowed("retry cap (2) reached: explanation or skip only")
		}
		if p.Stage == "" {
			p.Stage = ep.Stage
		}
		if p.Stage != ep.Stage {
			return notAllowed("retry must target the episode stage %s", ep.Stage)
		}
	case KindClarifyReply:
		if s.clarifyOrigin == nil {
			return notAllowed("no clarify request outstanding")
		}
	case KindExplore:
		if c == nil {
			return notAllowed("explore needs --concept")
		}
		if c.Verify == VerifySupported {
			return notAllowed("explore is for contested/unverifiable/intent_guess concepts")
		}
		if c.ExploreCount >= 2 {
			return notAllowed("explore capped at 2: give explore feedback and settle on open")
		}
	case KindOwnWords:
		if !s.AwaitOwnWords {
			return notAllowed("own_words only after explanation record (P1)")
		}
	case KindAwareness:
		if s.Source != SourceCode {
			return notAllowed("awareness questions belong to code mode")
		}
	case KindPrereq:
		if c == nil || !c.Provisional {
			return notAllowed("prereq_recovery targets a provisional concept")
		}
	default:
		return notAllowed("unknown question kind %q", k)
	}
	return nil
}

func (s *Session) noteAskSideEffects(p *AskParams) {
	ep := s.Episode
	switch p.Kind {
	case KindNarrow, KindHint:
		if ep != nil && ep.Open {
			ep.ScaffoldCount++
			ep.Scaffolded = true
			ep.Await = "" // outcome of the scaffold decides the next mandatory step
			c := s.Concept(ep.ConceptID)
			if c != nil {
				c.SupportEvents = append(c.SupportEvents, string(p.Kind))
			}
		}
	case KindRetry:
		if ep != nil && ep.Open {
			ep.Await = ""
		}
	case KindOwnWords:
		s.ExplanationOpen = false
	case KindBoundary, KindTransfer:
		if s.AwaitNewCase && p.NewCase {
			s.AwaitNewCase = false
		}
	case KindExplore:
		if c := s.Concept(p.ConceptID); c != nil {
			c.ExploreCount++
		}
	}
}

func (s *Session) lastClarityExplicit(conceptID string) bool {
	if s.PostRecheckClarity == ClarityExplicit {
		return true
	}
	for i := len(s.Judgments) - 1; i >= 0; i-- {
		j := s.Judgments[i]
		if j.ConceptID != conceptID || j.Tainted {
			continue
		}
		return j.ModelClarity == ClarityExplicit
	}
	return false
}

func (c *Concept) evidence(id string) *Evidence {
	for i := range c.Evidence {
		if c.Evidence[i].ID == id {
			return &c.Evidence[i]
		}
	}
	return nil
}

func (s *Session) openInvestigation() *Investigation {
	for i := range s.Investigations {
		if s.Investigations[i].Open {
			return &s.Investigations[i]
		}
	}
	return nil
}

// clarify state is transient session state (not persisted fields on Session
// JSON — it survives via replay of answer events).
type clarifyState struct {
	Orig  Question
	Count int
}

// Extra non-exported fields live on Session via a side struct to keep
// state.json stable; they are reconstructed on replay.
func (s *Session) ensureRuntime() {
	if s.LeadingCount == nil {
		s.LeadingCount = map[string]int{}
	}
}

// AnswerQuestion validates and accepts a user response (I3/I4) and returns
// the waiter exit code (docs/spec/core.md §3).
func (s *Session) AnswerQuestion(a Answer) (int, error) {
	s.ensureRuntime()
	if s.Pending == nil {
		return 5, notAllowed("no pending question")
	}
	if a.QID != s.Pending.QID {
		return 5, notAllowed("qid %q does not match pending %q (I3)", a.QID, s.Pending.QID)
	}
	q := s.Pending
	s.Pending = nil

	switch a.Action {
	case AnswerAbort:
		s.Aborted = true
		s.Phase = PhaseDone
		for _, c := range s.Concepts {
			if !c.Finalized && !c.Deferred {
				c.AbortedMid = true
			}
		}
		return 4, nil
	case AnswerClarify:
		// a clarify on a clarify_reply escalates the SAME clarify chain
		// (2nd -> modality switch, 3rd -> discard as inaccessible)
		if s.clarifyOrigin == nil ||
			(q.Kind != KindClarifyReply && s.clarifyOrigin.Orig.QID != q.QID) {
			s.clarifyOrigin = &clarifyState{Orig: *q}
		}
		s.clarifyOrigin.Count++
		if s.clarifyOrigin.Count >= 3 {
			// discard as inaccessible; a fresh question is required
			s.recordDiscard(q, QInaccessible)
			s.clarifyOrigin = nil
		}
		return 2, nil
	case AnswerExplain:
		c := s.Concept(q.ConceptID)
		if c == nil {
			c = s.currentConceptObj()
		}
		if c != nil {
			c.SupportEvents = append(c.SupportEvents, "explanation_requested")
			c.ExplanationUsed = true
			if q.Stage != "" {
				c.ExplanationStage = q.Stage
			} else if s.Episode != nil {
				c.ExplanationStage = s.Episode.Stage
			}
		}
		s.explainRequested = true
		return 6, nil
	case AnswerObject:
		s.objectionPending = &Question{}
		*s.objectionPending = *q
		return 7, nil
	case AnswerSkip:
		c := s.Concept(q.ConceptID)
		if c == nil {
			c = s.currentConceptObj()
		}
		if c != nil {
			s.deferConcept(c)
		}
		return 8, nil
	case AnswerChoice:
		if len(q.Options) == 0 || a.Choice < 1 || a.Choice > len(q.Options) {
			s.Pending = q // keep the screen; invalid input
			return 5, notAllowed("choice out of range")
		}
		if q.Kind == KindSessionLimit {
			s.handleLimitChoice(a.Choice, q)
		}
		s.LastAnswer = nil
		s.lastQuestion = q
		return 0, nil
	case AnswerText, "":
		switch q.Kind {
		case KindOpen:
			s.OpenBaseline = a.Text
			s.Phase = PhaseLadder
			return 0, nil
		case KindReexplain:
			s.ReexplainDone = true
			s.reexplainText = a.Text
			return 0, nil
		case KindClarifyReply:
			// the clarified question's answer is judged as the original kind
			orig := s.clarifyOrigin
			if orig != nil {
				q2 := *q
				q2.Kind = orig.Orig.Kind
				q2.Stage = orig.Orig.Stage
				q2.ConceptID = orig.Orig.ConceptID
				s.lastQuestion = &q2
				s.clarifyOrigin = nil
			} else {
				s.lastQuestion = q
			}
			s.LastAnswer = &a
			return 0, nil
		default:
			s.LastAnswer = &a
			s.lastQuestion = q
			return 0, nil
		}
	default:
		s.Pending = q
		return 5, notAllowed("unknown answer action %q", a.Action)
	}
}

func (s *Session) handleLimitChoice(choice int, q *Question) {
	s.LimitChoicePending = false
	s.LimitChoiceHandled = true
	// options are, by convention: 1=continue 2=summary-close 3=switch-to-explanation
	switch choice {
	case 2:
		s.summaryClose = true
		s.Phase = PhaseClose
	case 3:
		if c := s.currentConceptObj(); c != nil {
			c.SupportEvents = append(c.SupportEvents, "explanation_requested")
			c.ExplanationUsed = true
			if s.Episode != nil {
				c.ExplanationStage = s.Episode.Stage
			}
		}
		s.explainRequested = true
	}
}

func (s *Session) currentConceptObj() *Concept {
	if s.CurrentConcept != "" {
		if c := s.Concept(s.CurrentConcept); c != nil {
			return c
		}
	}
	for _, c := range s.Concepts {
		if !c.Finalized && !c.Deferred && !c.Provisional {
			return c
		}
	}
	return nil
}

func (s *Session) recordDiscard(q *Question, quality QuestionQuality) {
	c := s.Concept(q.ConceptID)
	if c != nil {
		c.Incidents = append(c.Incidents, Incident{Type: "question_discarded", Ref: q.QID})
	}
	if q.NewCase {
		// the mandatory post-explanation new case died without judgment:
		// re-arm the P1 gate so the next attempt is still required
		s.AwaitNewCase = true
	}
	// burden was already counted at ask (P8); judged is untouched (I7)
}

func (s *Session) deferConcept(c *Concept) {
	c.Deferred = true
	// the explanation chain, if any, belonged to this concept's flow
	s.AwaitNewCase = false
	s.AwaitOwnWords = false
	s.ExplanationOpen = false
	if s.Episode != nil && s.Episode.Open && s.Episode.ConceptID == c.ID {
		s.Episode.Open = false
	}
	out := s.computeOutcome(c)
	out.LearningOutcome = OutcomeDeferred
	out.EndReason = EndSkipped
	c.Outcome = out
	c.Finalized = true
	s.advanceCurrent()
	s.maybeEnterIntegrate()
}

// Summary returns the allowed next actions, for `squiz status` and the AI.
func (s *Session) AllowedSummary() []string {
	var out []string
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	if s.Phase == PhaseDone {
		return []string{"(session done)"}
	}
	if s.Pending != nil {
		return []string{fmt.Sprintf("wait (question %s pending)", s.Pending.QID)}
	}
	if s.LastAnswer != nil {
		return []string{fmt.Sprintf("judge (answer to %s)", s.LastAnswer.QID)}
	}
	if len(s.PendingRestore) > 0 {
		return []string{fmt.Sprintf("restore %s (I2)", strings.Join(s.PendingRestore, ","))}
	}
	if s.NeedRecheck {
		return []string{"verify --recheck (I6)"}
	}
	if s.NeedRecheckFeedback {
		return []string{"feedback --kind recheck (P6)"}
	}
	if s.LimitChoicePending {
		return []string{"ask --kind session_limit_choice (P8)"}
	}
	switch s.Phase {
	case PhasePrep:
		add("concept add / verify / start")
	case PhaseOpen:
		add("ask --kind open")
	case PhaseLadder:
		if s.ExplanationOpen {
			add("ask --kind own_words (P1)")
			break
		}
		if s.AwaitOwnWords {
			add("ask --kind own_words (P1)")
			break
		}
		if s.AwaitNewCase {
			add("ask --kind boundary|transfer --new-case (P1)")
			break
		}
		if s.explainRequested {
			add("explanation record")
		}
		if ep := s.Episode; ep != nil && ep.Open {
			if ep.Await != "" {
				add("ask --kind %s (P1 fading)", ep.Await)
			} else {
				add("episode: narrow/hint (scaffold %d/3) or retry (%d/2), episode close", ep.ScaffoldCount, ep.RetryCount)
			}
			break
		}
		if c := s.currentConceptObj(); c != nil {
			add("ask --kind %s --concept %s", s.nextStageKind(c), c.ID)
		}
		for _, c := range s.Concepts {
			if c.TransferUnlocked && c.stage(StageTransfer).State != StagePass && !c.Finalized {
				add("ask --kind transfer --concept %s (unlocked)", c.ID)
			}
			if s.readyToFinalize(c) {
				if !c.FeedbackDone {
					add("feedback --kind concept --concept %s (P6, before finalize)", c.ID)
				} else {
					add("finalize --concept %s", c.ID)
				}
			}
		}
	case PhaseIntegrate:
		add("ask --kind integrate (%d/2) or close", s.IntegrateCount)
	case PhaseClose:
		add("ask --kind reexplain, then close")
	}
	return out
}

func (s *Session) nextStageKind(c *Concept) QuestionKind {
	for _, st := range Ladder[:3] { // transfer is deferred
		if c.stage(st).State != StagePass {
			return QuestionKind(st)
		}
	}
	if c.TransferUnlocked {
		return KindTransfer
	}
	return KindBoundary
}

func (s *Session) readyToFinalize(c *Concept) bool {
	if c.Finalized || c.Provisional {
		return false
	}
	for _, st := range Ladder[:3] {
		if c.stage(st).State != StagePass {
			return false
		}
	}
	return c.stage(StageTransfer).State == StagePass || !c.TransferUnlocked
}
