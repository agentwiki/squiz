// Package e2e drives the real squiz binary through full user scenarios:
// a scripted AI agent issues the CLI commands exactly as SKILL.md
// prescribes, and a scripted user answers through the same file IPC the
// TUI uses (atomic rename into q/response.json). No engine internals are
// touched — every assertion goes through the binary's stdout, exit codes,
// and the files a real session produces.
//
// Each scenario also appends a sequential natural-language narrative with
// captured output evidence to a markdown report (SQUIZ_E2E_REPORT), which
// CI posts as a PR comment so reviewers can evaluate the scenarios as
// acceptance criteria, independent of the code diff.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binPath string

type section struct {
	title    string
	criteria []string // acceptance criteria, verified by the steps below
	steps    []string
	failed   bool
}

var sections []*section

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "squiz-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)
	binPath = filepath.Join(tmp, "squiz")
	build := exec.Command("go", "build", "-o", binPath, "../cmd/squiz")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	writeReport()
	os.Exit(code)
}

func writeReport() {
	path := os.Getenv("SQUIZ_E2E_REPORT")
	if path == "" {
		path = "e2e-report.md"
	}
	var b strings.Builder
	passed := 0
	for _, s := range sections {
		if !s.failed {
			passed++
		}
	}
	fmt.Fprintf(&b, "## 🧪 squiz E2E 사용자 시나리오 검증 — %d/%d 통과\n\n", passed, len(sections))
	b.WriteString("모의 AI 튜터가 실제 `squiz` 바이너리에 CLI 명령을 발행하고, 모의 사용자가 TUI와 동일한 파일 IPC로 답합니다. " +
		"아래 전개는 테스트가 실행 중 캡처한 실제 입출력이며, 각 시나리오의 수용 기준(AC)을 코드와 무관하게 검증합니다.\n\n")
	for _, s := range sections {
		mark := "✅"
		if s.failed {
			mark = "❌"
		}
		fmt.Fprintf(&b, "### %s %s\n\n", mark, s.title)
		b.WriteString("**수용 기준**\n")
		for _, c := range s.criteria {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n<details><summary><b>전개 (캡처된 실제 입출력)</b></summary>\n\n")
		for i, st := range s.steps {
			fmt.Fprintf(&b, "%d. %s\n", i+1, st)
		}
		b.WriteString("\n</details>\n\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "report write failed:", err)
	}
}

// ---- scenario runner --------------------------------------------------------

type run struct {
	t   *testing.T
	dir string
	sec *section
}

func newRun(t *testing.T, title string, criteria ...string) *run {
	r := &run{t: t, dir: t.TempDir(), sec: &section{title: title, criteria: criteria}}
	sections = append(sections, r.sec)
	t.Cleanup(func() { r.sec.failed = t.Failed() })
	return r
}

func (r *run) step(format string, a ...any) {
	r.sec.steps = append(r.sec.steps, fmt.Sprintf(format, a...))
}

func (r *run) evidence(label, content string) {
	content = strings.TrimSpace(content)
	r.sec.steps = append(r.sec.steps,
		fmt.Sprintf("📋 **%s**\n   ```json\n   %s\n   ```", label, strings.ReplaceAll(content, "\n", "\n   ")))
}

// squiz runs the binary in the scenario directory.
func (r *run) squiz(stdin string, args ...string) (stdout, stderr string, code int) {
	r.t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = r.dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code = 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			r.t.Fatalf("squiz %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

// ai runs a command the AI tutor would issue and requires success.
func (r *run) ai(desc, stdin string, args ...string) string {
	r.t.Helper()
	out, errS, code := r.squiz(stdin, args...)
	if code != 0 {
		r.step("🤖 %s → ❌ 실패 (exit %d): %s", desc, code, strings.TrimSpace(errS))
		r.t.Fatalf("AI command failed: squiz %v (exit %d)\nstderr: %s", args, code, errS)
	}
	r.step("🤖 AI: %s — `squiz %s`", desc, strings.Join(args, " "))
	return out
}

// aiRejected runs a command expecting the state machine to refuse it (I8):
// the refusal itself is acceptance evidence.
func (r *run) aiRejected(desc string, args ...string) {
	r.t.Helper()
	_, errS, code := r.squiz("", args...)
	if code == 0 {
		r.step("🛑 %s → 예상과 달리 허용됨", desc)
		r.t.Fatalf("expected rejection for squiz %v, got success", args)
	}
	msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(errS), "error:"))
	r.step("🛑 시스템 거부(의도된 동작): %s\n   > `%s`", desc, msg)
}

// screen polls the IPC question file the TUI would render.
func (r *run) screen() map[string]any {
	r.t.Helper()
	pattern := filepath.Join(r.dir, ".squiz", "sessions", "*", "q", "question.json")
	deadline := time.Now().Add(5 * time.Second)
	for {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			b, err := os.ReadFile(matches[0])
			if err == nil && len(b) > 0 {
				var sc map[string]any
				if json.Unmarshal(b, &sc) == nil {
					return sc
				}
			}
		}
		if time.Now().After(deadline) {
			r.t.Fatal("no question published to the user screen")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *run) queueDir() string {
	matches, _ := filepath.Glob(filepath.Join(r.dir, ".squiz", "sessions", "*", "q"))
	if len(matches) == 0 {
		r.t.Fatal("no session queue dir")
	}
	return matches[0]
}

// respond writes the user's response the way the TUI does: temp + rename.
func (r *run) respond(userLine string, ans map[string]any) {
	r.t.Helper()
	sc := r.screen()
	q := sc["question"].(map[string]any)
	ans["qid"] = q["qid"]

	head := "squiz"
	if n, _ := sc["concept_name"].(string); n != "" {
		head = n
		if st, _ := sc["stage"].(string); st != "" {
			head += " · " + st
		}
	}
	r.step("🖥️ 사용자 화면 `[%s]`: “%s”", head, q["text"])
	r.step("👤 사용자: %s", userLine)

	b, _ := json.Marshal(ans)
	qd := r.queueDir()
	tmp := filepath.Join(qd, "response.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		r.t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(qd, "response.json")); err != nil {
		r.t.Fatal(err)
	}
}

// wait runs `squiz wait` and checks the exit code contract (core.md §3).
func (r *run) wait(expectCode int) string {
	r.t.Helper()
	out, errS, code := r.squiz("", "wait", "--timeout", "10s")
	if code != expectCode {
		r.t.Fatalf("wait exit = %d, want %d\nstdout: %s\nstderr: %s", code, expectCode, out, errS)
	}
	if expectCode != 0 {
		r.step("⚙️ waiter exit %d — 자율성 경로로 분기", expectCode)
	}
	return out
}

// judge submits the AI's judgment and logs the verdict + next allowed set.
func (r *run) judge(verdict string, j map[string]any) string {
	r.t.Helper()
	if _, ok := j["question_quality"]; !ok {
		j["question_quality"] = "valid"
	}
	b, _ := json.Marshal(j)
	out := r.ai("판정 "+verdict, string(b), "judge")
	var res struct {
		Allowed []string `json:"allowed"`
	}
	json.Unmarshal([]byte(out), &res)
	r.step("⚙️ 판정 결과 `%s` → 상태 기계가 허용한 다음 동작: `%s`", verdict, strings.Join(res.Allowed, " | "))
	return out
}

// qa is one full screen: ask -> user answers -> wait(0) -> judge.
func (r *run) qa(askDesc string, askArgs []string, userLine string, answer, judgment map[string]any, verdict string) {
	r.t.Helper()
	r.ai(askDesc, "", append([]string{"ask"}, askArgs...)...)
	r.respond(userLine, answer)
	r.wait(0)
	r.judge(verdict, judgment)
}

func (r *run) jsonField(out string, path ...string) any {
	r.t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		r.t.Fatalf("bad JSON output: %v\n%s", err, out)
	}
	var cur any = v
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			r.t.Fatalf("path %v not found in %s", path, out)
		}
		cur = m[p]
	}
	return cur
}

func (r *run) assertEq(label string, got, want any) {
	r.t.Helper()
	g, w := fmt.Sprint(got), fmt.Sprint(want)
	if g != w {
		r.step("❌ 검증 실패: %s = `%s` (기대: `%s`)", label, g, w)
		r.t.Fatalf("%s = %v, want %v", label, got, want)
	}
	r.step("✔️ 검증: %s = `%s`", label, g)
}

// prep runs the standard preparation: init(ai), concepts, verify, start, open.
func (r *run) prep(concepts [][2]string, openAnswer string) {
	r.t.Helper()
	r.ai("세션 생성 (source=ai: 진리의 원천은 실행 결과)", "", "init", "--source", "ai")
	for i, c := range concepts {
		r.ai(fmt.Sprintf("개념 등록: %s", c[0]),
			`{"target_performance":"실무 적용","depth":"사용"}`,
			"concept", "add", "--name", c[0], "--claim", c[1], "--claim-type", "behavior")
		ev := fmt.Sprintf(`{"evidence":[{"kind":"execution","text":"반례 실행 완료: %s","excludes":"소박한 오개념"}]}`, c[1])
		out := r.ai("반례를 실제 실행해 claim 검증(supported)", ev,
			"verify", "--concept", fmt.Sprintf("c%d", i+1), "--status", "supported")
		_ = out
	}
	r.ai("세션 시작 (모든 개념 verify=supported 확인됨)", "", "start")
	r.ai("open 질문 등록 — “첫 답은 판정하지 않고 난이도·용어 맞추는 데만 씁니다”", "",
		"ask", "--kind", "open", "--text", "이 개념을 편한 방식으로 설명해 보세요")
	r.respond(fmt.Sprintf("(baseline) “%s”", openAnswer),
		map[string]any{"action": "answer", "text": openAnswer, "confidence": "unsure"})
	r.wait(0)
	r.step("⚙️ baseline만 기록 — open 답변은 판정하지 않음 (설계 §4.1)")
}

func alignedMech() map[string]any {
	return map[string]any{"alignment": "aligned", "support": "mechanism", "model_clarity": "explicit_prediction"}
}

func sure(text string) map[string]any {
	return map[string]any{"action": "answer", "text": text, "confidence": "sure"}
}
func unsure(text string) map[string]any {
	return map[string]any{"action": "answer", "text": text, "confidence": "unsure"}
}

// passStage asks one ladder question and passes it.
func (r *run) passStage(kind, concept, question, answer string) {
	r.qa(kind+" 질문 등록", []string{"--kind", kind, "--concept", concept, "--text", question},
		fmt.Sprintf("“%s” (확신)", answer), sure(answer), alignedMech(),
		"aligned/mechanism — rubric 충족")
}
