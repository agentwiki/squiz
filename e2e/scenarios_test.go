package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// S1 — 독립 수행 경로: 사용자가 도움 없이 사다리를 완주하면 demonstrated,
// transfer는 다음 개념 뒤로 지연된다.
func TestS1SelfReachedWithDeferredTransfer(t *testing.T) {
	r := newRun(t, "S1. 스스로 도달한 사용자 — 독립 수행과 transfer 지연",
		"사용자가 단서 없이 예측→근거→경계를 통과하면 시스템은 어떤 개입도 하지 않는다",
		"개념 A의 transfer(전이) 질문은 개념 B의 boundary가 끝나기 전에는 **시스템이 거부**한다 (패턴 추종 방지)",
		"네 단계를 모두 스스로 통과한 개념은 `demonstrated` + `self_reached`로 자동 판정되고, 보고 문구는 “이 세션에서 독립적으로 설명·적용함”이다",
		"유지력(retention)은 항상 `untested` — 한 세션으로 장기 기억을 주장하지 않는다 (P2)")

	r.prep([][2]string{
		{"결제 멱등성 키", "같은 키의 결제 요청 2회는 레코드 1개와 캐시 응답을 만든다"},
		{"재시도 백오프", "지수 백오프는 재시도 간격을 배수로 늘려 서버 부하 집중을 막는다"},
	}, "같은 요청이 두 번 와도 결제는 한 번만 되게 하는 장치라고 알고 있어요")

	r.passStage("predict", "c1", "같은 키로 결제 요청이 두 번 오면 레코드 수와 두 번째 응답은?",
		"레코드 1개, 두 번째는 첫 응답과 동일한 캐시 응답")
	r.passStage("why", "c1", "왜 키 하나로 부작용이 한 번만 일어나는지 과정 순서대로 설명하면?",
		"서버가 키를 저장해두고 같은 키가 오면 처리 대신 저장된 결과를 반환하기 때문")
	r.passStage("boundary", "c1", "어떤 조건이 바뀌면 이 보장이 깨지고, 왜?",
		"키 보관 기간이 만료되면 같은 키도 새 요청으로 처리되어 중복이 생긴다")

	// AC: A의 transfer는 아직 잠겨 있어야 한다
	r.aiRejected("개념 A(c1)의 transfer를 지금 물으려는 시도",
		"ask", "--kind", "transfer", "--concept", "c1",
		"--text", "이메일 발송에 옮기면 어떤 요소가 대응되나?")

	r.passStage("predict", "c2", "재시도 간격이 1s에서 시작해 3회 실패하면 4번째 대기 시간은?",
		"지수라면 8초 — 1,2,4 다음이니까")
	r.passStage("why", "c2", "왜 간격을 배수로 늘리는 게 부하 집중을 막지?",
		"실패 직후 동시 재시도가 몰리는 걸 시간축으로 흩뿌려서 피크를 낮추기 때문")
	r.passStage("boundary", "c2", "백오프가 오히려 해가 되는 조건은, 왜?",
		"장애가 이미 복구됐는데 대기가 길면 복구 후에도 불필요하게 지연된다")

	// B.boundary 이후에야 A.transfer가 열린다
	r.passStage("transfer", "c1", "이메일 발송에 옮기면 결제의 어떤 요소가 어디 대응되고 안 되는 건?",
		"키↔메시지ID, 부작용↔발송, 캐시 응답↔이미 보냈다는 응답. 본문 변경은 대응 안 됨")

	r.ai("개념 A 종료 피드백 — 확인된 것/범위/미결 (P6)", "",
		"feedback", "--kind", "concept", "--concept", "c1",
		"--text", "확인: 키 매칭으로 부작용 1회. 범위: 같은 키+같은 본문. 미결: 키 만료 정책")
	out := r.ai("개념 A 자동 판정(finalize) — AI가 결과를 찍지 않는다", "", "finalize", "--concept", "c1")
	r.evidence("c1 finalize 출력(자동 계산된 학습 결과)", extract(out, "outcome"))
	r.assertEq("c1 learning_outcome", r.jsonField(out, "outcome", "learning_outcome"), "demonstrated")
	r.assertEq("c1 acquisition_path", r.jsonField(out, "outcome", "acquisition_path"), "self_reached")
	r.assertEq("c1 retention (P2)", r.jsonField(out, "outcome", "retention"), "untested")

	r.passStage("transfer", "c2", "이 구조를 메시지 큐 소비자 재시도에 옮기면?",
		"실패↔nack, 간격 배수↔재전달 지연, 대응 안 되는 건 DLQ 이동 기준")
	r.ai("개념 B 종료 피드백", "", "feedback", "--kind", "concept", "--concept", "c2",
		"--text", "확인: 지수 백오프의 부하 분산 원리. 미결: 지터 필요성")
	r.ai("개념 B finalize", "", "finalize", "--concept", "c2")

	// integrate (두 개념 demonstrated이므로 진입)
	r.qa("통합 질문", []string{"--kind", "integrate", "--text", "멱등성 키와 재시도 백오프는 어떻게 맞물리나?"},
		"“백오프가 재시도를 만들고, 멱등성 키가 그 재시도를 무해하게 만든다” (확신)",
		sure("백오프가 재시도를 만들고 멱등성 키가 그 재시도를 무해하게 만든다"), alignedMech(), "aligned")
	r.qa("통합 질문 2", []string{"--kind", "integrate", "--text", "키 없이 백오프만 있으면 무슨 일이?"},
		"“중복 부작용이 그대로 남는다” (확신)", sure("중복 부작용이 남는다"), alignedMech(), "aligned")

	r.ai("reexplain — 첫 open 질문 재시행", "", "ask", "--kind", "reexplain",
		"--text", "처음 질문을 다시: 이 개념을 설명해 보세요")
	r.respond("“키 매칭으로 부작용을 1회로 보장하고, 백오프로 재시도를 분산한다”",
		map[string]any{"action": "answer", "text": "키 매칭으로 부작용 1회 보장, 백오프로 재시도 분산", "confidence": "sure"})
	r.wait(0)

	out = r.ai("세션 종료 — 보고 생성", "", "close")
	r.evidence("close 보고(발췌)", extract(out, "concepts"))
	var rep struct {
		Concepts []struct {
			Phrase string `json:"phrase"`
		} `json:"concepts"`
	}
	json.Unmarshal([]byte(out), &rep)
	r.assertEq("c1 학습자 보고 문구", rep.Concepts[0].Phrase, "이 세션에서 독립적으로 설명·적용함")
}

// S2 — 오개념+확신 경로 (설계 명세의 예시 궤적): recheck 강제, 피드백 게이트,
// scaffold 페이딩을 거쳐 guided로 도달.
func TestS2GuidedThroughMisconception(t *testing.T) {
	r := newRun(t, "S2. 강한 오개념 + 높은 확신 — recheck 강제와 scaffold 페이딩",
		"사용자가 **확신하며 반대**하면 시스템은 AI 판정을 믿지 않고 원천(실행) 재확인을 강제한다 (I6)",
		"재확인이 끝나면 **확인된 결과를 근거와 함께 말하기 전까지** 다음 질문이 차단된다 (P6 피드백 게이트)",
		"단서(narrow) 통과 후에는 재조합(recombine)→**단서 없는 재시도(retry)**가 강제되고, 그 재시도의 통과만 단계 통과로 기록된다 (P1)",
		"오개념 귀속은 체크리스트 전항 충족 시에만 — 결과는 `demonstrated`이되 경로는 `guided`로 구분 기록된다 (P1/P3)")

	r.prep([][2]string{
		{"결제 멱등성 키", "같은 키의 결제 요청 2회는 레코드 1개와 캐시 응답을 만든다"},
	}, "두 번 요청하면 두 번 결제되는 걸 막는 뭔가가 있다고 들었어요")

	// 오개념을 확신으로 답변
	r.ai("predict 질문 등록", "", "ask", "--kind", "predict", "--concept", "c1",
		"--text", "같은 키로 결제 요청이 두 번 들어오면, 레코드 수와 두 번째 응답은?")
	r.respond("“레코드 2개, 둘 다 성공 응답” (확신)", sure("레코드 2개, 둘 다 성공 응답"))
	r.wait(0)
	r.judge("contradicted + sure → recheck 강제(I6)", map[string]any{
		"alignment": "contradicted", "support": "convention", "model_clarity": "explicit_prediction"})

	r.aiRejected("recheck 없이 다음 질문을 하려는 시도(I6)",
		"ask", "--kind", "narrow", "--concept", "c1", "--text", "차원을 줄여볼까?")

	out := r.ai("원천 재확인: 반례를 다시 실행 — claim이 맞았음",
		`{"evidence":[{"kind":"execution","text":"같은 키 2회 실행 → 레코드 1, 두 번째는 캐시 응답","excludes":"두 번 결제된다"}]}`,
		"verify", "--concept", "c1", "--recheck", "--supports-claim")
	evID := lastEvidenceID(t, out)

	r.aiRejected("확인 결과를 말하지 않고 질문하려는 시도(P6 게이트)",
		"ask", "--kind", "consequence", "--concept", "c1", "--evidence", evID, "--text", "갈리는 가정은?")
	r.ai("피드백: 확인된 결과를 근거와 함께 제시 (P6)", "", "feedback", "--kind", "recheck",
		"--text", "확인된 실행에서는 레코드 1개, 두 번째는 첫 응답과 동일. 네 예측과 갈린다")

	r.qa("consequence — 검증된 반례를 근거로 갈리는 가정을 묻기 (I1)",
		[]string{"--kind", "consequence", "--concept", "c1", "--evidence", evID,
			"--text", "네 모델에서 실행 결과와 갈리는 가정은 어느 것일까?"},
		"“서버가 두 요청을 구분 못 한다는 가정이요” (불확실)",
		unsure("서버가 두 요청을 구분 못 한다는 가정"),
		map[string]any{"alignment": "partial", "support": "mechanism", "model_clarity": "vague",
			"partial_credit": "구분 여부가 핵심이라는 점"},
		"partial — 맞은 부분 명시(P7) 후 좁히기")

	r.qa("narrow — 답을 암시하지 않고 차원만 축소",
		[]string{"--kind", "narrow", "--concept", "c1",
			"--text", "서버가 두 요청이 같다는 걸 알 수 있는 정보가 요청 어딘가에 있나?"},
		"“헤더의 키...?” (불확실)", unsure("헤더의 키"),
		map[string]any{"alignment": "aligned", "support": "mechanism"}, "aligned")

	r.aiRejected("narrow 통과 후 recombine을 건너뛰고 힌트를 주려는 시도(P1)",
		"ask", "--kind", "hint", "--concept", "c1", "--level", "2", "--text", "힌트")

	r.qa("recombine — 조각을 원래 질문에 재조합",
		[]string{"--kind", "recombine", "--concept", "c1",
			"--text", "그 키로 서버가 하는 일을 처음 질문에 다시 넣어 설명하면?"},
		"“같은 키면 이전 결과를 돌려주니 레코드 1, 응답 동일” (확신)",
		sure("같은 키면 이전 결과를 돌려주니 레코드 1, 응답 동일"),
		map[string]any{"alignment": "aligned", "support": "mechanism"}, "aligned → 단서 없는 retry 강제")

	r.qa("retry — 단서 없는 새 사례 (이 통과만 stage final)",
		[]string{"--kind", "retry", "--concept", "c1",
			"--text", "첫 요청이 서버에 도달도 못 했으면 두 번째 요청은?"},
		"“키가 처음이니 정상 처리, 레코드 1” (확신)",
		sure("키가 처음이니 정상 처리, 레코드 1"), alignedMech(),
		"aligned/mechanism — predict=pass(guided)")

	r.ai("에피소드 종료 — user_misconception은 체크리스트 전항 충족 시에만 (P3)",
		`{"question_valid":true,"claim_evidence_valid":true,"evidence_excludes_model":true,"not_terminology":true,"not_prerequisite":true,"model_clear":true}`,
		"episode", "close", "--cause", "user_misconception")

	r.passStage("why", "c1", "왜 키 하나로 부작용이 한 번만 일어나지? 과정 순서대로",
		"키 저장 → 조회 일치 → 처리 생략, 저장된 응답 반환")
	r.passStage("boundary", "c1", "어떤 조건이 바뀌면 깨지고, 왜?",
		"키 만료 후엔 같은 키도 새 요청 — 저장된 매칭 대상이 없어져서")
	r.passStage("transfer", "c1", "이메일 발송에 옮기면 어떤 요소가 어디 대응되고 안 되는 건?",
		"키↔메시지ID, 부작용↔발송, 캐시↔이미 보냈다는 응답. 금액 같은 본문 변경은 대응 안 됨")

	r.ai("개념 종료 피드백 (P6)", "", "feedback", "--kind", "concept", "--concept", "c1",
		"--text", "확인: 키 매칭으로 부작용 1회. 범위: 같은 키+본문. 미결: 키 만료")
	out = r.ai("finalize — 자동 판정", "", "finalize", "--concept", "c1")
	r.evidence("finalize 출력(자동 계산)", extract(out, "outcome"))
	r.assertEq("learning_outcome", r.jsonField(out, "outcome", "learning_outcome"), "demonstrated")
	r.assertEq("acquisition_path (지원 구분)", r.jsonField(out, "outcome", "acquisition_path"), "guided")
	r.assertEq("recheck_recommended", r.jsonField(out, "outcome", "recheck_recommended"), true)
	r.assertEq("단일 개념 transfer 간격 없음 명시", r.jsonField(out, "outcome", "transfer_gap_note"), true)
}

// S3 — 모른다고 말하는 사용자: 설명 요청은 불이익이 없고(P5), 설명 뒤에는
// 자기 말 재구성과 새 사례가 강제된다(P1). 이후 사다리가 재개된다.
func TestS3ExplanationPath(t *testing.T) {
	r := newRun(t, "S3. 솔직히 모른다는 사용자 — 설명 요청 경로 (P5/P1)",
		"사용자의 설명 요청(/explain)은 즉시 수용되며 **불이익이 없다**: end_reason은 completed, 요청은 support_events에만 기록된다 (P5)",
		"설명 직후에는 자기 말 재구성(own_words) 외 어떤 질문도 **시스템이 거부**한다 (P1)",
		"재구성 후에는 **새 사례**(--new-case) boundary/transfer만 허용되고, 그 통과는 after_explanation 경로로 기록된다",
		"새 사례 통과 후 사다리가 재개되어 남은 단계를 계속 진행한다")

	r.prep([][2]string{
		{"결제 멱등성 키", "같은 키의 결제 요청 2회는 레코드 1개와 캐시 응답을 만든다"},
	}, "잘 모르겠어요. 결제 관련 안전장치인 것 같은데...")

	r.ai("predict 질문 등록", "", "ask", "--kind", "predict", "--concept", "c1",
		"--text", "같은 키로 결제 요청이 두 번 오면 레코드 수와 두 번째 응답은?")
	r.respond("“정말 모르겠어요” (모르겠음)",
		map[string]any{"action": "answer", "text": "모르겠어요", "confidence": "dontknow"})
	r.wait(0)
	r.judge("unknown — 모른다는 정상 경로, 단서/예시/설명 선택 제시", map[string]any{"alignment": "unknown"})

	r.ai("narrow 질문으로 작은 단서 제시 시도", "", "ask", "--kind", "narrow", "--concept", "c1",
		"--text", "요청 두 개가 서버 입장에서 같은지 다른지 판단할 정보가 있을까?")
	r.respond("사용자가 설명을 요청 — “/explain 기초부터 듣고 싶어요”",
		map[string]any{"action": "explain", "reason": "기초부터 듣고 싶어요"})
	r.wait(6)

	r.ai("설명 기록 — 요청은 support_events로만 기록됨", "",
		"explanation", "record", "--concept", "c1",
		"--text", "서버는 요청의 키를 저장하고, 같은 키가 다시 오면 처리 대신 저장된 결과를 돌려준다. 그래서 부작용은 1회다")

	r.aiRejected("설명 직후 predict를 다시 물으려는 시도(P1: own_words 먼저)",
		"ask", "--kind", "predict", "--concept", "c1", "--text", "그럼 다시, 두 번 오면?")

	r.qa("own_words — 자기 말 재구성 요구",
		[]string{"--kind", "own_words", "--concept", "c1", "--text", "방금 설명의 핵심을 당신 말로 다시 설명하면?"},
		"“키가 열쇠고 서버가 열쇠 구멍을 기억해서, 같은 열쇠는 두 번 안 돌아가는 거네요” (불확실)",
		unsure("키가 열쇠고 서버가 기억해서 같은 열쇠는 두 번 안 돌아간다"),
		map[string]any{"alignment": "aligned", "support": "mechanism"},
		"aligned/mechanism — 문구 반복이 아닌 재구성")

	r.aiRejected("새 사례 플래그 없이 boundary를 물으려는 시도(P1)",
		"ask", "--kind", "boundary", "--concept", "c1", "--text", "언제 깨지나?")

	r.qa("새 사례 boundary (--new-case)",
		[]string{"--kind", "boundary", "--concept", "c1", "--new-case",
			"--text", "새 사례: 키 보관함이 1시간마다 비워진다면 이 보장은 언제, 왜 깨지나?"},
		"“1시간 넘게 지나 재시도하면 같은 키도 새 요청 — 기억이 지워졌으니까” (확신)",
		sure("1시간 지나면 같은 키도 새 요청, 기억이 지워져서"), alignedMech(),
		"aligned/mechanism — boundary=pass(after_explanation)")

	// 설명 이후 사다리 재개
	r.passStage("predict", "c1", "재개: 같은 키 두 번, 레코드 수와 두 번째 응답은?",
		"레코드 1, 두 번째는 저장된 첫 응답")
	r.passStage("why", "c1", "왜 그렇게 되는지 순서대로?",
		"키 저장 → 일치 조회 → 처리 생략, 저장 응답 반환")
	r.passStage("transfer", "c1", "이메일 발송에 옮기면?",
		"키↔메시지ID, 부작용↔발송. 본문 변경은 대응 안 됨")

	r.ai("개념 종료 피드백", "", "feedback", "--kind", "concept", "--concept", "c1",
		"--text", "확인: 키 매칭 원리. 범위: 보관 기간 내. 미결: 만료 정책 실제 값")
	out := r.ai("finalize", "", "finalize", "--concept", "c1")
	r.evidence("finalize 출력", extract(out, "outcome"))
	r.assertEq("acquisition_path", r.jsonField(out, "outcome", "acquisition_path"), "after_explanation")
	r.assertEq("end_reason — 설명 요청은 실패가 아님(P5)", r.jsonField(out, "outcome", "end_reason"), "completed")
	sup := fmt2(r.jsonField(out, "outcome", "support_events"))
	if !strings.Contains(sup, "explanation_requested") {
		r.step("❌ support_events에 explanation_requested 없음: %s", sup)
		t.Fatalf("support_events missing explanation_requested: %s", sup)
	}
	r.step("✔️ 검증: support_events에 `explanation_requested` 기록됨 — %s", sup)
}

// S4 — AI가 틀린 경우: 사용자 확신 유지 → recheck가 claim을 반증 →
// 철회·소급 재판정으로 사용자가 복권된다. AI 오류는 예외가 아니라 핵심 경로.
func TestS4AIErrorRetractRestore(t *testing.T) {
	r := newRun(t, "S4. AI가 틀렸다 — 철회와 소급 재판정 (I2/I5/I6)",
		"사용자가 확신하며 반대하면 시스템은 사용자가 아니라 **AI의 claim을 먼저 의심**한다: 원천 재확인이 강제된다 (I6)",
		"재확인이 claim을 반증하면 **철회(retract) 전까지 모든 질문이 차단**된다",
		"철회는 그 claim을 참조한 기존 판정을 전부 오염(taint) 처리하고, **복구(restore) 재판정 외 어떤 동작도 허용하지 않는다** (I2)",
		"재판정에서 사용자의 원답이 옳았으면 단계가 소급 통과되고, incidents에 claim_retracted가 남으며 재확인이 권고된다")

	r.prep([][2]string{
		{"캐시 무효화 시점", "쓰기 직후 캐시를 지우므로 읽기는 항상 최신 값을 본다"},
	}, "쓰기 후에도 잠깐은 이전 값이 보일 수 있다고 알고 있는데요")

	r.ai("predict 질문 등록", "", "ask", "--kind", "predict", "--concept", "c1",
		"--text", "쓰기 완료 직후 다른 클라이언트가 읽으면 어떤 값을 보나?")
	r.respond("“복제 지연 동안은 이전 값을 볼 수 있어요. 확실합니다” (확신)",
		sure("복제 지연 동안은 이전 값을 볼 수 있다"))
	r.wait(0)
	r.judge("contradicted + sure → 사용자 답이 아니라 원천을 다시 본다(I6)", map[string]any{
		"alignment": "contradicted", "support": "mechanism", "model_clarity": "explicit_prediction"})

	out := r.ai("원천 재확인: 실행해 보니 **사용자가 옳았다** — claim 반증",
		`{"evidence":[{"kind":"execution","text":"쓰기 직후 병렬 읽기 실행 → 200ms간 이전 값 관측","excludes":"항상 최신 값"}]}`,
		"verify", "--concept", "c1", "--recheck")
	_ = out

	r.aiRejected("철회 없이 계속 질문하려는 시도",
		"ask", "--kind", "why", "--concept", "c1", "--text", "왜?")

	out = r.ai("claim 철회 — 버전 상승, 의존 판정 오염(I5)", "", "retract", "--concept", "c1")
	r.evidence("retract 출력 — 재판정 대기열", out)
	pr := fmt2(r.jsonField(out, "pending_restore"))

	r.aiRejected("오염 복구 전 다른 질문을 하려는 시도(I2: restore만 허용)",
		"ask", "--kind", "predict", "--concept", "c1", "--text", "다시 물을게")

	qid := strings.Trim(strings.Split(strings.Trim(pr, "[]"), " ")[0], `"`)
	r.ai("소급 재판정: 새 claim 기준으로 사용자 원답을 다시 판정 — aligned",
		`{"qid":"`+qid+`","alignment":"aligned","support":"mechanism","model_clarity":"explicit_prediction","question_quality":"valid","rationale":"사용자의 원답이 확인된 실행 결과와 일치"}`,
		"restore")

	out = r.ai("상태 확인 — predict가 소급 통과되었는지", "", "status")
	var st struct {
		Concepts []struct {
			Stages map[string]string `json:"stages"`
		} `json:"concepts"`
	}
	json.Unmarshal([]byte(out), &st)
	r.assertEq("restore 후 predict 단계", st.Concepts[0].Stages["predict"], "pass")

	r.ai("에피소드 원인 확정: ai_error — 사용자 오개념으로 세지 않는다 (P3)", "",
		"episode", "close", "--cause", "ai_error")

	r.passStage("why", "c1", "왜 잠깐 이전 값이 보일 수 있는지 과정 순서대로?",
		"쓰기는 주 노드에, 읽기는 복제본에서 — 복제 전파가 끝나기 전까지 이전 값")
	r.passStage("boundary", "c1", "어떤 조건에서는 항상 최신 값을 보게 되나, 왜?",
		"주 노드에서 읽거나 read-your-writes 세션 보장을 쓰면 — 전파를 기다리지 않으니까")
	r.passStage("transfer", "c1", "CDN 캐시 퍼지에 옮기면?",
		"원본 갱신↔쓰기, 엣지 TTL↔복제 지연. 대응 안 되는 건 수동 퍼지 API")

	r.ai("개념 종료 피드백", "", "feedback", "--kind", "concept", "--concept", "c1",
		"--text", "확인: 복제 지연 동안 이전 값 관측(당초 내 claim이 틀렸음). 미결: 지연 상한")
	out = r.ai("finalize", "", "finalize", "--concept", "c1")
	r.evidence("finalize 출력", extract(out, "outcome"))
	r.assertEq("learning_outcome", r.jsonField(out, "outcome", "learning_outcome"), "demonstrated")
	r.assertEq("recheck_recommended (오염 이력)", r.jsonField(out, "outcome", "recheck_recommended"), true)
	inc := fmt2(r.jsonField(out, "outcome", "incidents"))
	if !strings.Contains(inc, "claim_retracted") {
		t.Fatalf("incidents missing claim_retracted: %s", inc)
	}
	r.step("✔️ 검증: incidents에 `claim_retracted` 기록 — AI 오류와 학습 결과가 별도 차원으로 남음")
}

// ---- small helpers ----------------------------------------------------------

func fmt2(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// extract pretty-prints one top-level field of a JSON document.
func extract(out string, field string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return out
	}
	raw, ok := m[field]
	if !ok {
		return out
	}
	var buf strings.Builder
	var v any
	json.Unmarshal(raw, &v)
	b, _ := json.MarshalIndent(v, "", "  ")
	buf.Write(b)
	return buf.String()
}

func lastEvidenceID(t *testing.T, verifyOut string) string {
	t.Helper()
	var v struct {
		Evidence []struct {
			ID string `json:"id"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &v); err != nil || len(v.Evidence) == 0 {
		t.Fatalf("verify output has no evidence ids: %s", verifyOut)
	}
	return v.Evidence[len(v.Evidence)-1].ID
}
