// Package engine implements the squiz state machine: the reduce(state, event)
// specified by squiz-transitions.md (v4.1) and the invariants I1-I8 / P1-P8
// from squiz-design.md and squiz-core.md. The CLI and TUI are thin shells
// around this package; every rule the docs mark as CLI-enforced lives here.
package engine

import "encoding/json"

type Source string

const (
	SourceAI      Source = "ai"
	SourceConcept Source = "concept"
	SourceCode    Source = "code"
)

type Role string

const (
	RoleNone     Role = ""
	RoleAuthor   Role = "author"
	RoleReviewer Role = "reviewer"
)

type Phase string

const (
	PhasePrep      Phase = "prep"
	PhaseOpen      Phase = "open"
	PhaseLadder    Phase = "ladder"
	PhaseIntegrate Phase = "integrate"
	PhaseClose     Phase = "close"
	PhaseDone      Phase = "done"
)

type Stage string

const (
	StagePredict  Stage = "predict"
	StageWhy      Stage = "why"
	StageBoundary Stage = "boundary"
	StageTransfer Stage = "transfer"
)

// Ladder is the stage order within one concept. Transfer is deferred across
// concepts (D35); see Session.nextLadderTarget.
var Ladder = []Stage{StagePredict, StageWhy, StageBoundary, StageTransfer}

// StageState: A3 resolved — no "fail" terminal state. Exhausted stages stay
// pending ("evidence not obtained this session").
type StageState string

const (
	StagePending    StageState = "pending"
	StageScaffolded StageState = "scaffolded"
	StagePass       StageState = "pass"
)

type Alignment string

const (
	Aligned      Alignment = "aligned"
	Contradicted Alignment = "contradicted"
	Partial      Alignment = "partial"
	Divergent    Alignment = "divergent"
	Unknown      Alignment = "unknown"
	Mismatch     Alignment = "mismatch"
)

type Support string

const (
	SupMechanism    Support = "mechanism"
	SupEvidence     Support = "evidence"
	SupConvention   Support = "convention"
	SupNone         Support = "none"
	SupNotRequested Support = "not_requested"
)

type ModelClarity string

const (
	ClarityExplicit ModelClarity = "explicit_prediction"
	ClarityVague    ModelClarity = "vague"
	ClarityNone     ModelClarity = "none"
)

type QuestionQuality string

const (
	QValid          QuestionQuality = "valid"
	QAmbiguous      QuestionQuality = "ambiguous"
	QLeading        QuestionQuality = "leading"
	QCompound       QuestionQuality = "compound"
	QUnderspecified QuestionQuality = "underspecified"
	QOffClaim       QuestionQuality = "off_claim"
	QPrereqMissing  QuestionQuality = "prerequisite_missing"
	QInaccessible   QuestionQuality = "inaccessible"
)

type Confidence string

const (
	Sure     Confidence = "sure"
	Unsure   Confidence = "unsure"
	DontKnow Confidence = "dontknow"
)

type Cause string

const (
	CauseUndetermined       Cause = "undetermined"
	CauseUserMisconception  Cause = "user_misconception"
	CauseQuestionDefect     Cause = "question_defect"
	CauseAIError            Cause = "ai_error"
	CauseTerminology        Cause = "terminology"
	CauseWeakCounterexample Cause = "weak_counterexample"
	CausePrerequisiteGap    Cause = "prerequisite_gap"
)

type ClaimType string

const (
	ClaimBehavior ClaimType = "behavior"
	ClaimIntent   ClaimType = "intent"
	ClaimFact     ClaimType = "fact"
	ClaimNorm     ClaimType = "norm"
)

type VerifyStatus string

const (
	VerifyPending      VerifyStatus = "pending"
	VerifySupported    VerifyStatus = "supported"
	VerifyRefuted      VerifyStatus = "refuted"
	VerifyUnverifiable VerifyStatus = "unverifiable"
	VerifyContested    VerifyStatus = "contested"
	VerifyIntentGuess  VerifyStatus = "intent_guess"
)

type QuestionKind string

const (
	KindOpen         QuestionKind = "open"
	KindPredict      QuestionKind = "predict"
	KindWhy          QuestionKind = "why"
	KindBoundary     QuestionKind = "boundary"
	KindTransfer     QuestionKind = "transfer"
	KindConsequence  QuestionKind = "consequence"
	KindNarrow       QuestionKind = "narrow"
	KindRecombine    QuestionKind = "recombine"
	KindRetry        QuestionKind = "retry"
	KindModelClarify QuestionKind = "model_clarify"
	KindHint         QuestionKind = "hint"
	KindClarifyReply QuestionKind = "clarify_reply"
	KindExplore      QuestionKind = "explore"
	KindOwnWords     QuestionKind = "own_words"
	KindIntegrate    QuestionKind = "integrate"
	KindReexplain    QuestionKind = "reexplain"
	KindAwareness    QuestionKind = "awareness"
	KindPrereq       QuestionKind = "prereq_recovery"
	KindSessionLimit QuestionKind = "session_limit_choice"
)

// ladderKinds map 1:1 onto stages; retry also targets a stage.
var ladderKinds = map[QuestionKind]Stage{
	KindPredict: StagePredict, KindWhy: StageWhy,
	KindBoundary: StageBoundary, KindTransfer: StageTransfer,
}

type AcquisitionPath string

const (
	PathSelfReached      AcquisitionPath = "self_reached"
	PathGuided           AcquisitionPath = "guided"
	PathAfterExplanation AcquisitionPath = "after_explanation"
	PathStopped          AcquisitionPath = "stopped"
)

type LearningOutcome string

const (
	OutcomeDemonstrated    LearningOutcome = "demonstrated"
	OutcomePartial         LearningOutcome = "partial"
	OutcomeNotDemonstrated LearningOutcome = "not_demonstrated"
	OutcomeOpen            LearningOutcome = "open"
	OutcomeDeferred        LearningOutcome = "deferred"
)

type EndReason string

const (
	EndCompleted  EndReason = "completed"
	EndSkipped    EndReason = "skipped"
	EndAborted    EndReason = "aborted"
	EndLimit      EndReason = "limit"
	EndUnresolved EndReason = "unresolved"
)

// Action is the set of operations the state machine can allow next (I8).
type Action string

const (
	ActConceptAdd    Action = "concept_add"
	ActVerify        Action = "verify"
	ActStart         Action = "start"
	ActAsk           Action = "ask"
	ActAnswer        Action = "answer"
	ActJudge         Action = "judge"
	ActFeedback      Action = "feedback"
	ActEpisodeClose  Action = "episode_close"
	ActExplanation   Action = "explanation_record"
	ActRecheck       Action = "recheck"
	ActRetract       Action = "retract"
	ActRestore       Action = "restore"
	ActDefect        Action = "defect"
	ActFinding       Action = "finding"
	ActInvestigation Action = "investigation"
	ActFinalize      Action = "finalize"
	ActSkip          Action = "skip"
	ActClose         Action = "close"
	ActDiscard       Action = "discard_question"
)

type Claim struct {
	Text    string    `json:"text"`
	Type    ClaimType `json:"type"`
	Version int       `json:"version"`
}

type Evidence struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // execution | external_ref | thought_experiment
	Text      string `json:"text"`
	Excludes  string `json:"excludes,omitempty"`
	Version   int    `json:"version"`
	Retracted bool   `json:"retracted,omitempty"`
}

// Judgment is the AI's structured judgment of one answer (design §4.3).
type Judgment struct {
	QID             string          `json:"qid"`
	Alignment       Alignment       `json:"alignment"`
	Support         Support         `json:"support"`
	ModelClarity    ModelClarity    `json:"model_clarity"`
	QuestionQuality QuestionQuality `json:"question_quality"`
	Confidence      Confidence      `json:"confidence"` // echoed from the answer
	PartialCredit   string          `json:"partial_credit,omitempty"`
	Misconception   string          `json:"misconception,omitempty"`
	ClaimVersion    int             `json:"claim_version"`
	EvidenceRefs    []string        `json:"evidence_refs,omitempty"`
	Rationale       string          `json:"rationale,omitempty"`
}

// JudgmentRecord ties a judgment to its question context for replay,
// taint tracking (I5) and stage recomputation after restore.
type JudgmentRecord struct {
	Judgment
	ConceptID  string       `json:"concept_id"`
	Kind       QuestionKind `json:"kind"`
	Stage      Stage        `json:"stage,omitempty"`
	Scaffolded bool         `json:"scaffolded"` // answered inside an episode after scaffolding
	Tainted    bool         `json:"tainted,omitempty"`
	Restored   bool         `json:"restored,omitempty"`
}

type Question struct {
	QID          string       `json:"qid"`
	Kind         QuestionKind `json:"kind"`
	ConceptID    string       `json:"concept_id,omitempty"`
	Stage        Stage        `json:"stage,omitempty"`
	Text         string       `json:"text"`
	HintLevel    int          `json:"hint_level,omitempty"`
	Options      []string     `json:"options,omitempty"` // structured-choice questions
	ClarifyCount int          `json:"clarify_count,omitempty"`
	EvidenceRef  string       `json:"evidence_ref,omitempty"` // consequence: verified evidence (I1)
}

// AnswerAction: what the user did with the pending question in the TUI.
// Maps to waiter exit codes (squiz-core.md §3).
type AnswerAction string

const (
	AnswerText    AnswerAction = "answer"  // exit 0
	AnswerClarify AnswerAction = "clarify" // exit 2
	AnswerAbort   AnswerAction = "abort"   // exit 4
	AnswerExplain AnswerAction = "explain" // exit 6
	AnswerObject  AnswerAction = "object"  // exit 7
	AnswerSkip    AnswerAction = "skip"    // exit 8
	AnswerChoice  AnswerAction = "choice"  // structured selection, exit 0
)

type Answer struct {
	QID        string       `json:"qid"`
	Action     AnswerAction `json:"action"`
	Text       string       `json:"text,omitempty"`
	Confidence Confidence   `json:"confidence,omitempty"`
	Choice     int          `json:"choice,omitempty"` // 1-based index into Question.Options
	Reason     string       `json:"reason,omitempty"` // e.g. explanation-request reason (D31)
}

type Incident struct {
	Type string `json:"type"` // defect_found | finding | claim_retracted | evidence_retracted | question_discarded
	Ref  string `json:"ref"`
}

type StageInfo struct {
	State  StageState      `json:"state"`
	Path   AcquisitionPath `json:"path,omitempty"` // how pass was reached
	Reasks int             `json:"reasks"`         // aligned-but-insufficient re-ask count (max 1)
}

type Outcome struct {
	LearningOutcome    LearningOutcome `json:"learning_outcome"`
	AcquisitionPath    AcquisitionPath `json:"acquisition_path"`
	SessionConsistency string          `json:"session_consistency"` // consistent | inconsistent
	Retention          string          `json:"retention"`           // always "untested" in-session (P2)
	RecheckRecommended bool            `json:"recheck_recommended"`
	EndReason          EndReason       `json:"end_reason"`
	SupportEvents      []string        `json:"support_events"`
	Incidents          []Incident      `json:"incidents"`
	Vindicated         []string        `json:"vindicated_propositions"`
	TransferGapNote    bool            `json:"transfer_gap_note,omitempty"` // A1: single-concept, no delay gap
}

type Concept struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Claim        Claim        `json:"claim"`
	Verify       VerifyStatus `json:"verify"`
	ExternalRefs []string     `json:"external_refs,omitempty"`
	Evidence     []Evidence   `json:"evidence,omitempty"`

	TargetPerformance      string   `json:"target_performance,omitempty"`
	Depth                  string   `json:"depth,omitempty"`
	AcceptedAlternatives   []string `json:"accepted_alternatives,omitempty"`
	Prerequisites          []string `json:"prerequisites,omitempty"`
	ExpectedMisconceptions []string `json:"expected_misconceptions,omitempty"`

	Stages map[Stage]*StageInfo `json:"stages"`

	MisconceptionEpisodes int        `json:"misconception_episodes"`
	WhyReturns            int        `json:"why_returns"`
	SupportEvents         []string   `json:"support_events,omitempty"`
	Incidents             []Incident `json:"incidents,omitempty"`
	Vindicated            []string   `json:"vindicated,omitempty"`
	TaintContrib          bool       `json:"taint_contrib,omitempty"`
	ExploreCount          int        `json:"explore_count,omitempty"`

	ExplanationUsed  bool  `json:"explanation_used,omitempty"`
	ExplanationStage Stage `json:"explanation_stage,omitempty"`
	OwnWordsPassed   bool  `json:"own_words_passed,omitempty"`
	NewCaseDone      bool  `json:"new_case_done,omitempty"`      // post-explanation new case passed
	NewCaseAttempted bool  `json:"new_case_attempted,omitempty"` // post-explanation new case tried (A4)

	TransferUnlocked    bool     `json:"transfer_unlocked,omitempty"` // D35 gating
	FeedbackDone        bool     `json:"feedback_done,omitempty"`     // P6 concept feedback
	ExploreFeedbackDone bool     `json:"explore_feedback_done,omitempty"`
	Finalized           bool     `json:"finalized,omitempty"`
	Deferred            bool     `json:"deferred,omitempty"`    // skipped
	Paused              bool     `json:"paused,omitempty"`      // prerequisite_gap downstream pause
	Provisional         bool     `json:"provisional,omitempty"` // A5 lightweight prerequisite registration
	Outcome             *Outcome `json:"outcome,omitempty"`
	AbortedMid          bool     `json:"aborted_mid,omitempty"`
}

// Episode is one branch episode (divergence from the ladder until return).
type Episode struct {
	ConceptID     string `json:"concept_id"`
	Stage         Stage  `json:"stage"`
	ScaffoldCount int    `json:"scaffold_count"` // narrow+hint questions, <=3
	RetryCount    int    `json:"retry_count"`    // A2: <=2
	Cause         Cause  `json:"cause"`
	Open          bool   `json:"open"`
	Scaffolded    bool   `json:"scaffolded"` // any scaffold used in this episode
	// Await forces the P1 fading chain: after narrow aligned only recombine,
	// after recombine aligned only retry, after hint aligned only retry.
	Await        QuestionKind `json:"await,omitempty"`
	ReadyToClose bool         `json:"ready_to_close,omitempty"`
}

type Investigation struct {
	ID         string `json:"id"`
	ConceptID  string `json:"concept_id"`
	Reason     string `json:"reason"`
	Open       bool   `json:"open"`
	Resolution string `json:"resolution,omitempty"` // supported | retracted | unresolved
}

// MisconceptionChecklist gates cause=user_misconception (P3).
// Every field must be true.
type MisconceptionChecklist struct {
	QuestionValid         bool `json:"question_valid"`
	ClaimEvidenceValid    bool `json:"claim_evidence_valid"`
	EvidenceExcludesModel bool `json:"evidence_excludes_model"`
	NotTerminology        bool `json:"not_terminology"`
	NotPrerequisite       bool `json:"not_prerequisite"`
	ModelClear            bool `json:"model_clear"`
}

func (c MisconceptionChecklist) AllTrue() bool {
	return c.QuestionValid && c.ClaimEvidenceValid && c.EvidenceExcludesModel &&
		c.NotTerminology && c.NotPrerequisite && c.ModelClear
}

// Event is the persisted unit; events.jsonl is the source of truth.
type Event struct {
	Seq     int             `json:"seq"`
	TS      string          `json:"ts"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Session is the full state. Serialized as state.json (a cache; the event
// log wins on divergence).
type Session struct {
	ID       string     `json:"id"`
	Source   Source     `json:"source"`
	Role     Role       `json:"role"`
	Phase    Phase      `json:"phase"`
	Concepts []*Concept `json:"concepts"`

	CurrentConcept string    `json:"current_concept,omitempty"`
	Pending        *Question `json:"pending,omitempty"`
	LastAnswer     *Answer   `json:"last_answer,omitempty"` // awaiting judge
	OpenBaseline   string    `json:"open_baseline,omitempty"`

	Episode        *Episode         `json:"episode,omitempty"`
	Judgments      []JudgmentRecord `json:"judgments"`
	Investigations []Investigation  `json:"investigations,omitempty"`

	JudgedCount int `json:"judged_count"`
	BurdenCount int `json:"burden_count"`

	PendingRestore []string `json:"pending_restore,omitempty"` // qids needing re-judgment (I2)

	NeedRecheck         bool         `json:"need_recheck,omitempty"` // I6
	RecheckConcept      string       `json:"recheck_concept,omitempty"`
	NeedRecheckFeedback bool         `json:"need_recheck_feedback,omitempty"` // P6
	PostRecheckClarity  ModelClarity `json:"post_recheck_clarity,omitempty"`

	ExplanationOpen    bool `json:"explanation_open,omitempty"`
	AwaitOwnWords      bool `json:"await_own_words,omitempty"`
	AwaitNewCase       bool `json:"await_new_case,omitempty"`
	OwnWordsRepeatUsed bool `json:"own_words_repeat_used,omitempty"`

	LimitChoicePending bool `json:"limit_choice_pending,omitempty"` // P8 burden gate
	LimitChoiceHandled bool `json:"limit_choice_handled,omitempty"`
	IntegrateCount     int  `json:"integrate_count,omitempty"`
	ReexplainDone      bool `json:"reexplain_done,omitempty"`
	Aborted            bool `json:"aborted,omitempty"`

	PrereqDepth int `json:"prereq_depth,omitempty"` // A5: recovery depth used (cap 1)

	LeadingCount map[string]int `json:"leading_count,omitempty"` // per concept: 2 -> investigation

	NextQID int `json:"next_qid"`
	NextEID int `json:"next_eid"` // evidence/investigation id counter
	Seq     int `json:"seq"`      // last applied event seq

	// Runtime fields are rebuilt by event replay (events.jsonl is the source
	// of truth; state.json is a cache) and intentionally not serialized.
	clarifyOrigin    *clarifyState
	explainRequested bool
	retractDue       string
	objectionPending *Question
	lastChoice       *Answer
	lastQuestion     *Question
	reexplainText    string
	summaryClose     bool
	newCasePending   bool
}

func (s *Session) Concept(id string) *Concept {
	for _, c := range s.Concepts {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (c *Concept) stage(st Stage) *StageInfo {
	if c.Stages == nil {
		c.Stages = map[Stage]*StageInfo{}
	}
	si, ok := c.Stages[st]
	if !ok {
		si = &StageInfo{State: StagePending}
		c.Stages[st] = si
	}
	return si
}

func (c *Concept) passCount() int {
	n := 0
	for _, st := range Ladder {
		if c.stage(st).State == StagePass {
			n++
		}
	}
	return n
}
