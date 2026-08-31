// Package cli implements the squiz subcommands. Output is JSON on stdout
// (the AI is the consumer); errors go to stderr with exit code 1, except
// `wait`, whose exit code is the response channel (squiz-core.md §3).
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agentwiki/squiz/internal/engine"
	"github.com/agentwiki/squiz/internal/ipc"
	"github.com/agentwiki/squiz/internal/store"
	"github.com/agentwiki/squiz/internal/tui"
)

var version = "dev" // set via -ldflags at release time

func SetVersion(v string) { version = v }

func Main(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	cmd, rest := args[0], args[1:]
	// two-word commands: concept add, episode close, explanation record,
	// investigation open/close
	if len(rest) > 0 {
		switch cmd + " " + rest[0] {
		case "concept add":
			cmd, rest = "concept-add", rest[1:]
		case "episode close":
			cmd, rest = "episode-close", rest[1:]
		case "explanation record":
			cmd, rest = "explanation-record", rest[1:]
		case "investigation open":
			cmd, rest = "investigation-open", rest[1:]
		case "investigation close":
			cmd, rest = "investigation-close", rest[1:]
		}
	}
	fn, ok := commands[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		return 1
	}
	code, err := fn(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

var commands = map[string]func([]string) (int, error){
	"init":               cmdInit,
	"concept-add":        cmdConceptAdd,
	"verify":             cmdVerify,
	"start":              cmdStart,
	"ask":                cmdAsk,
	"wait":               cmdWait,
	"ui":                 cmdUI,
	"judge":              cmdJudge,
	"feedback":           cmdFeedback,
	"episode-close":      cmdEpisodeClose,
	"explanation-record": cmdExplanation,
	"retract":            cmdRetract,
	"restore":            cmdRestore,
	"defect":             cmdDefect,
	"finding":            cmdFinding,
	"investigation-open": cmdInvOpen,
	"investigation-close": cmdInvClose,
	"objection":          cmdObjection,
	"finalize":           cmdFinalize,
	"skip":               cmdSkip,
	"close":              cmdClose,
	"status":             cmdStatus,
	"version":            cmdVersion,
	"help":               func([]string) (int, error) { usage(); return 0, nil },
}

func usage() {
	fmt.Fprint(os.Stderr, `squiz — Socratic understanding checker (state machine CLI)

session:   init | concept add | verify | start | status | close | version
questions: ask | wait | judge | feedback | ui
episodes:  episode close | explanation record | finalize | skip
ai errors: retract | restore | defect | finding | investigation open|close | objection

Run in a repo; state lives in ./.squiz. The learner answers in a separate
terminal running "squiz ui". See SKILL.md for the tutoring protocol.
`)
}

func openStore() (*store.Store, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return store.Open(wd)
}

func loadActive() (*store.Store, *engine.Session, error) {
	st, err := openStore()
	if err != nil {
		return nil, nil, err
	}
	s, err := st.LoadActive()
	if err != nil {
		return nil, nil, err
	}
	return st, s, nil
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func readStdinJSON(v any) error {
	fi, err := os.Stdin.Stat()
	if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		return errors.New("expected JSON on stdin")
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return errors.New("expected JSON on stdin")
	}
	return json.Unmarshal(b, v)
}

func maybeStdinJSON(v any) bool {
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) != 0 {
		return false
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

// ---- commands --------------------------------------------------------------

func cmdVersion([]string) (int, error) {
	fmt.Println("squiz", version)
	return 0, nil
}

func cmdInit(args []string) (int, error) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	src := fs.String("source", "", "ai|concept|code")
	role := fs.String("role", "", "author|reviewer (code mode)")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, err := openStore()
	if err != nil {
		return 1, err
	}
	s, err := st.Create(engine.Source(*src), engine.Role(*role))
	if err != nil {
		return 1, err
	}
	printJSON(map[string]any{"session": s.ID, "phase": s.Phase})
	return 0, nil
}

func cmdConceptAdd(args []string) (int, error) {
	fs := flag.NewFlagSet("concept add", flag.ContinueOnError)
	name := fs.String("name", "", "concept name")
	claim := fs.String("claim", "", "claim text")
	ctype := fs.String("claim-type", "behavior", "behavior|intent|fact|norm")
	provisional := fs.Bool("provisional", false, "A5 prerequisite registration")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	p := engine.ConceptAddPayload{}
	maybeStdinJSON(&p.ConceptParams) // target_performance, depth, ... (design §4.0)
	if *name != "" {
		p.Name = *name
	}
	if *claim != "" {
		p.ClaimText = *claim
	}
	if p.ClaimType == "" {
		p.ClaimType = engine.ClaimType(*ctype)
	}
	p.Provisional = *provisional
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvConceptAdd, p); err != nil {
		return 1, err
	}
	c := s.Concepts[len(s.Concepts)-1]
	printJSON(map[string]any{"concept": c.ID, "name": c.Name})
	return 0, nil
}

func cmdVerify(args []string) (int, error) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	status := fs.String("status", "", "supported|refuted|unverifiable|contested|intent_guess")
	recheck := fs.Bool("recheck", false, "I6 recheck of a standing claim")
	supports := fs.Bool("supports-claim", false, "recheck outcome: claim confirmed")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	p := engine.VerifyParams{}
	maybeStdinJSON(&p) // evidence[], external_refs[]
	p.ConceptID = *concept
	if *status != "" {
		p.Status = engine.VerifyStatus(*status)
	}
	p.Recheck = *recheck
	if *supports {
		p.RecheckSupportsClaim = true
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvVerify, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"concept": p.ConceptID, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdStart(args []string) (int, error) {
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvStart, struct{}{}); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"phase": s.Phase, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdAsk(args []string) (int, error) {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	kind := fs.String("kind", "", "question kind")
	concept := fs.String("concept", "", "concept id")
	stage := fs.String("stage", "", "predict|why|boundary|transfer (retry)")
	text := fs.String("text", "", "question text (one screen)")
	level := fs.Int("level", 0, "hint level 1..4")
	evidence := fs.String("evidence", "", "evidence id (consequence, I1)")
	newCase := fs.Bool("new-case", false, "post-explanation new case (P1)")
	options := fs.String("options", "", "comma-separated choice options")
	notice := fs.String("notice", "", "non-question context line (A7, not counted in burden)")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	p := engine.AskParams{
		Kind: engine.QuestionKind(*kind), ConceptID: *concept,
		Stage: engine.Stage(*stage), Text: *text, HintLevel: *level,
		EvidenceRef: *evidence, NewCase: *newCase,
	}
	if *options != "" {
		p.Options = strings.Split(*options, ",")
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvAsk, p); err != nil {
		return 1, err
	}
	q := s.Pending
	x, err := ipc.New(st.QueueDir(s.ID))
	if err != nil {
		return 1, err
	}
	sc := ipc.Screen{
		Question: q, Notice: *notice,
		JudgedCount: s.JudgedCount, BurdenCount: s.BurdenCount,
	}
	if c := s.Concept(q.ConceptID); c != nil {
		sc.ConceptName = c.Name
		sc.Stage = q.Stage
	}
	if err := x.PublishQuestion(sc); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"qid": q.QID, "kind": q.Kind, "burden": s.BurdenCount})
	return 0, nil
}

// exit codes per squiz-core.md §3
func exitCodeFor(a engine.Answer) int {
	switch a.Action {
	case engine.AnswerClarify:
		return 2
	case engine.AnswerAbort:
		return 4
	case engine.AnswerExplain:
		return 6
	case engine.AnswerObject:
		return 7
	case engine.AnswerSkip:
		return 8
	default:
		return 0
	}
}

func cmdWait(args []string) (int, error) {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 0, "0 = wait forever")
	if err := fs.Parse(args); err != nil {
		return 5, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 5, err
	}
	if s.Pending == nil {
		return 5, errors.New("no pending question")
	}
	x, err := ipc.New(st.QueueDir(s.ID))
	if err != nil {
		return 5, err
	}
	a, err := x.Wait(*timeout)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return 3, errors.New("timeout: question stays pending")
		}
		return 5, err
	}
	if err := st.Commit(s, engine.EvAnswer, *a); err != nil {
		// invalid response (qid mismatch etc.): drop it, keep waiting state
		return 5, err
	}
	_ = x.ClearQuestion()
	printJSON(map[string]any{
		"qid": a.QID, "action": a.Action, "text": a.Text,
		"confidence": a.Confidence, "choice": a.Choice, "reason": a.Reason,
		"allowed": s.AllowedSummary(),
	})
	return exitCodeFor(*a), nil
}

func cmdUI(args []string) (int, error) {
	st, err := openStore()
	if err != nil {
		return 1, err
	}
	id, err := st.ActiveID()
	if err != nil {
		return 1, errors.New("no active session; the AI runs `squiz init` first")
	}
	return tui.Run(st.QueueDir(id))
}

func cmdJudge(args []string) (int, error) {
	var j engine.Judgment
	if err := readStdinJSON(&j); err != nil {
		return 1, fmt.Errorf("judge reads the judgment JSON on stdin (design §4.3): %w", err)
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if j.QID == "" && s.LastAnswer != nil {
		j.QID = s.LastAnswer.QID
	}
	if err := st.Commit(s, engine.EvJudge, j); err != nil {
		return 1, err
	}
	printJSON(map[string]any{
		"judged": s.JudgedCount, "burden": s.BurdenCount,
		"warn":    s.JudgedCount >= 24,
		"allowed": s.AllowedSummary(),
	})
	return 0, nil
}

func cmdFeedback(args []string) (int, error) {
	fs := flag.NewFlagSet("feedback", flag.ContinueOnError)
	kind := fs.String("kind", "", "recheck|concept|explore (P6)")
	concept := fs.String("concept", "", "concept id")
	text := fs.String("text", "", "the confirmed result / scope / open items")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.FeedbackPayload{Kind: engine.FeedbackKind(*kind), ConceptID: *concept, Text: *text}
	if err := st.Commit(s, engine.EvFeedback, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdEpisodeClose(args []string) (int, error) {
	fs := flag.NewFlagSet("episode close", flag.ContinueOnError)
	cause := fs.String("cause", "undetermined", "episode cause (P0-6)")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	p := engine.EpisodeClosePayload{Cause: engine.Cause(*cause)}
	if engine.Cause(*cause) == engine.CauseUserMisconception {
		var cl engine.MisconceptionChecklist
		if err := readStdinJSON(&cl); err != nil {
			return 1, fmt.Errorf("user_misconception requires the checklist JSON on stdin (P3): %w", err)
		}
		p.Checklist = &cl
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvEpisodeClose, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"cause": p.Cause, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdExplanation(args []string) (int, error) {
	fs := flag.NewFlagSet("explanation record", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	text := fs.String("text", "", "the explanation given")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.ExplanationPayload{ConceptID: *concept, Text: *text}
	if err := st.Commit(s, engine.EvExplanation, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdRetract(args []string) (int, error) {
	fs := flag.NewFlagSet("retract", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	kind := fs.String("kind", "claim", "claim|evidence")
	ref := fs.String("ref", "", "evidence id (kind=evidence)")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.RetractPayload{ConceptID: *concept, Kind: *kind, Ref: *ref}
	if err := st.Commit(s, engine.EvRetract, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"pending_restore": s.PendingRestore, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdRestore(args []string) (int, error) {
	var j engine.Judgment
	if err := readStdinJSON(&j); err != nil {
		return 1, fmt.Errorf("restore reads the re-judgment JSON on stdin: %w", err)
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvRestore, j); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"pending_restore": s.PendingRestore, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdDefect(args []string) (int, error) {
	fs := flag.NewFlagSet("defect", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	desc := fs.String("desc", "", "defect description")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	var body struct {
		Vindicated []string `json:"vindicated_propositions"`
	}
	if err := readStdinJSON(&body); err != nil {
		return 1, fmt.Errorf("defect requires vindicated_propositions JSON on stdin (P0-10): %w", err)
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.DefectPayload{ConceptID: *concept, Description: *desc, Vindicated: body.Vindicated}
	if err := st.Commit(s, engine.EvDefect, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"pending_restore": s.PendingRestore, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdFinding(args []string) (int, error) {
	fs := flag.NewFlagSet("finding", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	ref := fs.String("ref", "", "what was found")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvFinding, engine.FindingPayload{ConceptID: *concept, Ref: *ref}); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true})
	return 0, nil
}

func cmdInvOpen(args []string) (int, error) {
	fs := flag.NewFlagSet("investigation open", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	reason := fs.String("reason", "", "why the claim is in doubt")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.InvestigationPayload{Op: "open", ConceptID: *concept, Reason: *reason}
	if err := st.Commit(s, engine.EvInvestigation, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdInvClose(args []string) (int, error) {
	fs := flag.NewFlagSet("investigation close", flag.ContinueOnError)
	res := fs.String("resolution", "", "supported|retracted|unresolved")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.InvestigationPayload{Op: "close", Resolution: *res}
	if err := st.Commit(s, engine.EvInvestigation, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdObjection(args []string) (int, error) {
	fs := flag.NewFlagSet("objection", flag.ContinueOnError)
	discard := fs.Bool("discard", false, "discard the contested question (no judgment)")
	reason := fs.String("reason", "", "investigation reason otherwise")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	p := engine.ObjectionPayload{Discard: *discard, Reason: *reason}
	if err := st.Commit(s, engine.EvObjection, p); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"ok": true, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdFinalize(args []string) (int, error) {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvFinalize, engine.ConceptRefPayload{ConceptID: *concept}); err != nil {
		return 1, err
	}
	c := s.Concept(*concept)
	printJSON(map[string]any{"outcome": c.Outcome, "phase": s.Phase, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdSkip(args []string) (int, error) {
	fs := flag.NewFlagSet("skip", flag.ContinueOnError)
	concept := fs.String("concept", "", "concept id")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvSkip, engine.ConceptRefPayload{ConceptID: *concept}); err != nil {
		return 1, err
	}
	printJSON(map[string]any{"phase": s.Phase, "allowed": s.AllowedSummary()})
	return 0, nil
}

func cmdClose(args []string) (int, error) {
	st, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	if err := st.Commit(s, engine.EvClose, struct{}{}); err != nil {
		return 1, err
	}
	// recompute the report from the closed session for output
	rep := buildReport(s)
	printJSON(rep)
	return 0, nil
}

func buildReport(s *engine.Session) map[string]any {
	concepts := []map[string]any{}
	for _, c := range s.Concepts {
		if c.Provisional {
			continue
		}
		concepts = append(concepts, map[string]any{
			"id": c.ID, "name": c.Name, "outcome": c.Outcome,
		})
	}
	return map[string]any{
		"session": s.ID, "judged": s.JudgedCount, "burden": s.BurdenCount,
		"concepts": concepts,
		"recheck_advice": "다른 날 짧은 재확인 권장 (P2)",
	}
}

func cmdStatus(args []string) (int, error) {
	_, s, err := loadActive()
	if err != nil {
		return 1, err
	}
	concepts := []map[string]any{}
	for _, c := range s.Concepts {
		stages := map[string]string{}
		for _, stg := range engine.Ladder {
			stages[string(stg)] = string(c.Stages[stg].State)
		}
		concepts = append(concepts, map[string]any{
			"id": c.ID, "name": c.Name, "verify": c.Verify, "stages": stages,
			"finalized": c.Finalized, "deferred": c.Deferred,
			"transfer_unlocked": c.TransferUnlocked,
			"misconception_episodes": c.MisconceptionEpisodes,
		})
	}
	out := map[string]any{
		"session": s.ID, "phase": s.Phase, "source": s.Source,
		"judged_count": s.JudgedCount, "burden_count": s.BurdenCount,
		"judged_warn_24": s.JudgedCount >= 24,
		"concepts": concepts,
		"allowed":  s.AllowedSummary(),
	}
	if s.Pending != nil {
		out["pending"] = s.Pending.QID
	}
	if s.Episode != nil && s.Episode.Open {
		out["episode"] = s.Episode
	}
	if len(s.PendingRestore) > 0 {
		out["pending_restore"] = s.PendingRestore
	}
	printJSON(out)
	return 0, nil
}
