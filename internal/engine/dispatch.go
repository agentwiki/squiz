package engine

import (
	"encoding/json"
	"fmt"
)

// Event payloads. Every state change flows through Apply so that replaying
// events.jsonl reproduces the exact state (squiz-core.md §4).

type InitPayload struct {
	ID     string `json:"id"`
	Source Source `json:"source"`
	Role   Role   `json:"role"`
}

type ConceptAddPayload struct {
	ConceptParams
	Provisional bool `json:"provisional,omitempty"`
}

type FeedbackPayload struct {
	Kind      FeedbackKind `json:"kind"`
	ConceptID string       `json:"concept_id,omitempty"`
	Text      string       `json:"text"`
}

type EpisodeClosePayload struct {
	Cause     Cause                   `json:"cause"`
	Checklist *MisconceptionChecklist `json:"checklist,omitempty"`
}

type ExplanationPayload struct {
	ConceptID string `json:"concept_id,omitempty"`
	Text      string `json:"text"`
}

type RetractPayload struct {
	ConceptID string `json:"concept_id"`
	Kind      string `json:"kind"` // claim | evidence
	Ref       string `json:"ref,omitempty"`
}

type DefectPayload struct {
	ConceptID   string   `json:"concept_id"`
	Description string   `json:"description"`
	Vindicated  []string `json:"vindicated_propositions"`
}

type FindingPayload struct {
	ConceptID string `json:"concept_id"`
	Ref       string `json:"ref"`
}

type InvestigationPayload struct {
	Op         string `json:"op"` // open | close
	ConceptID  string `json:"concept_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type ObjectionPayload struct {
	Discard bool   `json:"discard"`
	Reason  string `json:"reason,omitempty"`
}

type ConceptRefPayload struct {
	ConceptID string `json:"concept_id"`
}

const (
	EvInit          = "init"
	EvConceptAdd    = "concept_add"
	EvVerify        = "verify"
	EvStart         = "start"
	EvAsk           = "ask"
	EvAnswer        = "answer"
	EvJudge         = "judge"
	EvFeedback      = "feedback"
	EvEpisodeClose  = "episode_close"
	EvExplanation   = "explanation_record"
	EvRetract       = "retract"
	EvRestore       = "restore"
	EvDefect        = "defect"
	EvFinding       = "finding"
	EvInvestigation = "investigation"
	EvObjection     = "objection"
	EvFinalize      = "finalize"
	EvSkip          = "skip"
	EvClose         = "close"
)

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Apply validates and applies one event. Used both for live commands and
// for replay; replay of a previously accepted log must not error.
func (s *Session) Apply(typ string, payload json.RawMessage) error {
	s.ensureRuntime()
	dec := func(v any) error {
		if len(payload) == 0 {
			return fmt.Errorf("empty payload for %s", typ)
		}
		return json.Unmarshal(payload, v)
	}
	switch typ {
	case EvInit:
		return fmt.Errorf("init is handled by NewSession")
	case EvConceptAdd:
		var p ConceptAddPayload
		if err := dec(&p); err != nil {
			return err
		}
		_, err := s.AddConcept(p.ConceptParams, p.Provisional)
		return err
	case EvVerify:
		var p VerifyParams
		if err := dec(&p); err != nil {
			return err
		}
		return s.SetVerify(p)
	case EvStart:
		return s.Start()
	case EvAsk:
		var p AskParams
		if err := dec(&p); err != nil {
			return err
		}
		_, err := s.Ask(p)
		return err
	case EvAnswer:
		var a Answer
		if err := dec(&a); err != nil {
			return err
		}
		_, err := s.AnswerQuestion(a)
		return err
	case EvJudge:
		var j Judgment
		if err := dec(&j); err != nil {
			return err
		}
		return s.Judge(j)
	case EvFeedback:
		var p FeedbackPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.Feedback(p.Kind, p.ConceptID, p.Text)
	case EvEpisodeClose:
		var p EpisodeClosePayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.EpisodeClose(p.Cause, p.Checklist)
	case EvExplanation:
		var p ExplanationPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.ExplanationRecord(p.ConceptID, p.Text)
	case EvRetract:
		var p RetractPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.Retract(p.ConceptID, p.Kind, p.Ref)
	case EvRestore:
		var j Judgment
		if err := dec(&j); err != nil {
			return err
		}
		return s.Restore(j)
	case EvDefect:
		var p DefectPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.Defect(p.ConceptID, p.Description, p.Vindicated)
	case EvFinding:
		var p FindingPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.Finding(p.ConceptID, p.Ref)
	case EvInvestigation:
		var p InvestigationPayload
		if err := dec(&p); err != nil {
			return err
		}
		if p.Op == "open" {
			return s.InvestigationOpen(p.ConceptID, p.Reason)
		}
		return s.InvestigationClose(p.Resolution)
	case EvObjection:
		var p ObjectionPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.ResolveObjection(p.Discard, p.Reason)
	case EvFinalize:
		var p ConceptRefPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.Finalize(p.ConceptID)
	case EvSkip:
		var p ConceptRefPayload
		if err := dec(&p); err != nil {
			return err
		}
		return s.SkipConcept(p.ConceptID)
	case EvClose:
		_, err := s.Close()
		return err
	}
	return fmt.Errorf("unknown event type %q", typ)
}
