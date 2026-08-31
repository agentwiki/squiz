# squiz — 소크라테스식 이해 점검 도구 설계 문서 (v4.1)

> 다른 AI 세션이 읽고 곧바로 재개할 수 있도록 작성되었다.
> 상태: **설계 v4.1, 구현 미착수**. 마지막 갱신: 2026-08-27.
> v3 → v4: 교수법 리뷰 반영. scaffold 페이딩 강제, 단계별 rubric, 유지력 명칭 분리, cause 기본값 `undetermined`, consequence 조건을 model_clarity로, 피드백 시점 정의, 설명 요청을 지원 사건으로, 이중 카운트, defect 복권 범위 제한, integration 진입 조건, transfer 지연, 교수법 불변조건 P1~P8, 미결 두 건 확정. 전이표는 `squiz-transitions.md`.
> v4 → v4.1: 교차 검증 수정. 오개념 3회 탈출에 skip 허용 통일(D38), integration 진입 조건의 "서로 다른 두 개념" 명시, 전이표 누락 행 보강(why S=none, 판정 24 경고). 결정 필요 항목은 `phase0a-decisions.md`로 분리.
> v4.1 확정분(0A, 2026-08-31, 사용자 승인): A1~A7 전부 확정(D39), 구현 언어 Go·TUI bubbletea·IPC 파일 기반·goreleaser 배포(D40). I1~I8·응답 승인·exit 코드·저장 구조는 **`squiz-core.md`(v3에서 재구성)**가 원천.
> 재개 시 §12(다음 작업)부터. §14는 등급표.

---

## 0. 한 문단 요약

`squiz`는 **Claude Code 스킬 + CLI(상태 기계) + TUI**다. 사용자가 어떤 대상(AI가 구현한 것, 일반 개념, 사람이 작성한 코드)을 정답 맞추기가 아니라 **소크라테스식 문답으로 스스로 이해에 도달**하게 만드는 **형성평가형 튜터**다. 규칙은 CLI가 강제한다. AI는 질문을 등록하고 백그라운드 대기자를 띄운 뒤 유휴 상태가 되며, 사용자가 별도 터미널의 TUI에서 답하면 깨어난다. **진리의 원천은 AI가 아니라 실행 결과(또는 외부 근거)이며 AI는 반례 공급자다.** AI 오류는 예외가 아니라 핵심 경로다. **도움받아 답한 것과 도움 없이 보여준 것은 구조적으로 구분된다.**

---

## 1. 목표와 비목표

### 목표
- 사용자가 **예측·근거·경계·전이**를 **scaffold 없이** 스스로 수행하도록 이끈다.
- 오답을 정정하지 않고 귀결을 묻는다. 탐색 후에는 근거에 연결된 피드백을 준다.
- 한 화면 한 질문.
- AI 결함·전제 오류는 사용자 이해 문제로 오인하지 않고 철회·무효화·소급 재판정한다.

### 비목표
- 점수·등급. 문제 은행. 웹 UI.
- **한 세션으로 장기 유지력을 판정하는 것.** 지연 확인 전까지 유지력은 "미검증"이다.
- 모든 개념의 완결. `open`은 정상 종결.

### 설계의 정체성 (수정 불가)
- AI claim도 가설이다.
- 사용자가 근거로 버티면 원천을 다시 본다.
- 질문이 잘못되면 사용자 답을 판정하지 않는다.
- 모름·설명 요청·이의 제기·skip·open은 정상 경로다.
- 예측·근거·경계·전이를 사용자 생성으로 확인한다.
- AI 오류와 학습 결과를 별도 차원으로 기록한다.
- 지원받은 답을 독립 수행으로 계산하지 않는다.

---

## 2. 용어

| 용어 | 뜻 |
|---|---|
| claim | AI의 가설. 버전 관리. `claim_type ∈ behavior | intent | fact | norm` (`norm` = 규약 자체가 학습 목표) |
| evidence | 실행 결과 또는 외부 근거. 버전 관리 |
| 사다리 | predict → why → boundary → transfer. transfer는 **다음 개념 뒤로 지연** |
| scaffold | narrow / hint / explanation. 이후 반드시 **unscaffolded retry** |
| 분기 에피소드 | 비정렬 판정에서 사다리 복귀까지의 질문 묶음. ≤3문항. `cause` 기록, 기본값 `undetermined` |
| model_clarity | 사용자 답이 관찰 가능한 예측을 내는 명료한 모델인지. consequence의 진짜 조건 |
| investigation | claim 단위 의심 조사 |
| taint | 철회된 버전을 참조한 판정의 오염 |
| restore | 원답을 새 claim 기준으로 재판정 |
| vindicated proposition | defect로 확인된 **사용자의 구체 명제 하나**. 단계·개념 통과가 아님 |
| 판정 문항 수 / 부담 문항 수 | 유효 학습 질문 수 / 사용자에게 보인 모든 화면 수(폐기 질문·clarify 포함) |
| waiter | `squiz wait` |

---

## 3. 뿌리 원칙

### 3.1 진리의 원천은 AI가 아니다
위험은 비대칭이 아니라 **잘못된 확신의 전이**다. 원천은 `ai`/`code`에서 실행, `concept`에서 외부 근거. AI claim은 가설. 검증되지 않은 반례로 모순 대면하지 않는다(I1). 판정 불가는 `open`.

### 3.2 공유된 맹점 (정직한 한계)
사용자 답과 AI 가설이 일치하면 시스템은 의심하지 않는다. `concept` 모드에서 특히. 완화는 외부 반례 선확보. 보고에 항상 명시.

### 3.3 이해 판정: 단계별 rubric
같은 `aligned + mechanism`이라도 단계마다 요구하는 증거가 다르다.

| 단계 | 통과 증거 | 불충분 |
|---|---|---|
| predict | 조건이 주어진 상황에서 **관찰 가능한 결과**를 명시 | "달라질 것 같다" |
| why | 결과를 낳는 **인과 관계·불변량·규칙** | `evidence`("실행해보니 그랬다")는 근거일 뿐 설명이 아님. `convention`은 `claim_type=norm`이고 출처·기능을 알 때만 충분 |
| boundary | 어떤 조건이 바뀌면 결과가 달라지는지 **와 그 이유** | 결과만 맞힘, 희귀 예외 나열 |
| transfer | 원 사례의 본질 구조 → 새 사례 요소 **대응**, 대응 안 되는 표면 요소 식별, 결과 예측 | "둘 다 중복을 막는다" |
| own_words | 설명의 핵심 관계를 자기 말로 재구성 | 설명 문구 반복, 키워드 나열 |

predict 답변이 인과를 포함하면 why 면제 가능하나, **why rubric으로 별도 판정**한 뒤에만.

### 3.4 개념 결과 모델
```json
{
  "learning_outcome": "demonstrated | partial | not_demonstrated | open | deferred",
  "acquisition_path": "self_reached | guided | after_explanation | stopped",
  "session_consistency": "consistent | inconsistent",
  "retention": "untested | delayed_demonstrated | delayed_failed",
  "recheck_recommended": true,
  "end_reason": "completed | skipped | aborted | limit | unresolved",
  "support_events": ["hint", "narrow", "explanation_requested"],
  "incidents": [ { "type": "defect_found | finding | claim_retracted | evidence_retracted | question_discarded", "ref": "..." } ],
  "vindicated_propositions": [ "..." ]
}
```
**자동 판정** (`concept finalize`, AI가 찍지 않음):
- `demonstrated`: 네 단계 final=pass **이고 각 final은 unscaffolded 시도**
- `partial`: 2~3단계. `not_demonstrated`: ≤1 ("모른다"가 아니라 "이 세션에서 증거 미확보")
- `acquisition_path`: scaffold 없이 → `self_reached`; narrow/hint 후 unscaffolded retry 통과 → `guided`; explanation 후 새 사례 통과 → `after_explanation`
- `session_consistency=inconsistent`: **`user_misconception`으로 확정된** 에피소드 ≥2 / transfer 실패 후 why 복귀 / reexplain 누락 / 오염 판정 기여
- `retention`: 세션 내 판정으로는 항상 `untested`. 지연 확인(Phase 2) 후에만 갱신. **`solid` 명칭 폐기**
- `recheck_recommended`: inconsistent ∨ guided/after_explanation ∨ 마지막 답 unsure(우선순위용)
- `end_reason`은 완료 여부만. 설명 요청은 `support_events`. **도움 요청은 결과를 낮추지 않는다(P5)**

학습자 보고 표현: demonstrated→"이 세션에서 독립적으로 설명·적용함", guided→"단서를 활용해 재구성함", after_explanation→"설명 후 새 사례에 적용함", not_demonstrated→"이번 대화에서는 아직 확인되지 않음", retention untested→"다른 날 짧은 재확인 권장". `misconception`은 내부 용어. 보고에서는 "초기 모델 / 재검토한 가정 / 범위를 수정한 설명".

### 3.5 소스 모드
`squiz init --source ai|concept|code [--role author|reviewer]`.

| | `ai` | `concept` | `code` |
|---|---|---|---|
| 원천 | 실행 | 외부 근거 | 실행. 의도는 작성자 |
| verify | supported/refuted/unverifiable | supported/contested/unverifiable | + intent_guess |
| supported 조건 | 반례 실행 + 오개념 배제 | 외부 URL/인용 + 문헌 반례 ≥1. 회상만이면 contested | 반례 실행 + 오개념 배제 |
| 반례 | 실행 입력 | 사고 실험(준비 단계 것만) | 실행 입력 |
| open 질문 | "핵심 결정 셋?" | "한 문단·예시·코드 흐름 중 편한 방식으로" | author "네가 내린 결정 셋?" / reviewer "작성자는 왜?" |
| 결함 | `defect` → 공동 수정 | 없음 | `finding`, 수정 안 함 |

verify 결과별: `supported` 전체 사다리+consequence / `contested`·`unverifiable`·`intent_guess` 사다리+explore, consequence 금지 / `refuted`·`pending` 시작 차단.

**`code --role author`**: predict=실제 동작(검증 대상), why=의도(권위, `contradicted` 불가), boundary=갈라지는 지점. 보고에서 `current_intent`와 `original_intent_recollection` 구분.

**미결 확정 (v3 §7 잔여)**
- `contested`에서 `contradicted+sure`: investigation open, consequence 계속 잠금, recheck 대상은 사용자 답이 아니라 claim과 양쪽 외부 근거. 판별 근거 없으면 즉시 unresolved → explore → open. **user_misconception으로 세지 않는다.** 나중에 새 근거가 나와도 소급해 오개념으로 찍지 않고 새 근거 제시 후 재판단 기회.
- author predict `divergent`: **investigation/recheck 우선**. 실행 결과가 사용자 예측 지지 → claim retract. 다르면 관찰 사실을 중립 제시 → 현재 의도/당시 의도 분리 질문 → 검증된 동작 ≠ 의도일 때만 `finding`. 질문: "확인된 실행 결과는 X, 말한 의도 Y와 다릅니다. 의도된 동작인가요, 예상 밖인가요, 근거를 더 봐야 하나요?"

---

## 4. 세션 흐름

### 4.0 준비
1. 개념 **기본 2~3, 최대 5**. 판정 문항 12~18 목표.
2. 개념당: claim(type 포함), **목표 수행(어디서 쓸 수 있어야 하나), 필요 깊이(사용/리뷰/설계/설명), 허용 대안 설명(문구가 달라도 인정할 모델)**, 예상 오개념, 반례, 의존, **필요 선행 개념**.
3. verify: 오개념 배제 증거(`--excludes`). 판정은 문구 일치가 아니라 핵심 관계·조건·예측의 보존.
4. 순서: 의존 순, 첫 개념은 쉬운 것.

### 4.1 개시
프레임: 내 것도 검토 대상 / 되물어도 됨 / 설명 요청·이의 제기·건너뛰기 가능 / **"첫 답은 판정하지 않고 난이도·용어 맞추는 데만 씁니다."** open 답변은 baseline만.

### 4.2 사다리와 지연 transfer
개념 A: predict → why → boundary. 그 다음 개념 B로 이동. **A의 transfer는 B의 boundary 이후**에 묻는다(패턴 추종 방지). 마지막 개념의 transfer는 integration 직전. 개념이 하나뿐이면 reexplain 직전 — 이때 시간 간격이 사실상 없으므로 **보고에 "지연 간격 없음" 한계를 명시**한다(A1, §3.2와 일관).

predict 질문은 초기 조건·변화 변수·관찰 결과·범위를 포함해야 한다. 조건 빠진 질문은 `underspecified`.
why 재질문은 "왜?" 반복이 아니라 형식을 바꾼다: 과정 순서 / 요소 제거 시 결과 / 중간 상태 / 두 설명 비교.
boundary 사례는 희귀 함정이 아니라 **적용 범위를 가르는 조건**.

### 4.3 판정 구조
```json
{
  "alignment": "aligned | contradicted | partial | divergent | unknown | mismatch",
  "support":   "mechanism | evidence | convention | none | not_requested",
  "model_clarity": "explicit_prediction | vague | none",
  "question_quality": "valid | ambiguous | leading | compound | underspecified | off_claim | prerequisite_missing | inaccessible",
  "confidence": "sure | unsure | dontknow",
  "partial_credit": "맞은 부분 명시 (partial일 때 필수, 모든 단계)",
  "misconception": null, "claim_version": 1, "evidence_refs": [], "rationale": "..."
}
```
- `question_quality ≠ valid` → 답변 판정 미사용, 질문 폐기·재작성(I7). `prerequisite_missing`은 선행 개념 복구로, `inaccessible`은 표현 양식 전환으로.
- **consequence 조건 (P0-7)**: `model_clarity=explicit_prediction` ∧ 검증된 반례가 그 예측을 배제 ∧ investigation 없음. **확신도는 조건이 아니다.** `sure`는 recheck 강제(I6)에만 쓴다.
- consequence 질문은 수사적이 아니라 **관찰 가능한 예측 요구**: "네 설명대로면 결제 레코드 수와 두 번째 응답은?" → 실행 결과 제시 → "예상과 갈리는 가정은?" ("그런 코드를 왜 넣었을까?"는 leading)
- `narrow`(차원 축소, 답 암시 없음) vs `leading`(답 암시·전제 삽입) 구분 명시.

**주요 전이** (전체는 transitions §1~2):

| 판정 | 다음 |
|---|---|
| aligned + rubric 충족 | 다음 단계 |
| aligned + convention/none (norm 아님) | 형식 바꾼 why 재질문 → narrow. consequence 금지 |
| divergent | investigation → recheck |
| contradicted + sure | recheck → (explicit_prediction이면) consequence, 아니면 모델 명료화 질문 |
| contradicted + explicit_prediction (unsure 포함) | consequence(검증됨) |
| contradicted + vague/none | 모델 명료화 질문 → narrow |
| contradicted + dontknow / unknown | narrow 또는 **사용자 선택 힌트 등급** / 설명 요청 능동 안내 |
| partial (모든 단계) | 맞은 부분 명시 인정 → 틀린 부분 고립 narrow |
| mismatch | 중립 awareness 질문 → finding |

**scaffold 페이딩 (P1, CLI 강제)**
- `narrow aligned` → **recombine 필수** → **unscaffolded retry**(같은 단계, 새 표현 또는 새 사례) → 그 결과만 stage final
- `hint aligned` → **unscaffolded retry** 필수
- `explanation` → `own_words` → **새 사례 boundary 또는 transfer** → **사다리 재개**(A4): 남은 pending 단계를 계속 진행하고, explanation이 다룬 단계만 after_explanation 경로로 기록. 부담 상한(P8) 도달 시에만 즉시 finalize. own_words가 설명 반복이면 재질문이 아니라 형식을 바꾼 적용 문제
- scaffold ≤3에 더해 **에피소드당 unscaffolded retry ≤2**(A2). 소진 시 allowed={explanation(escape), skip}
- 힌트 등급(가벼운 순): 주의 단서 / 사실 하나 / 반례 관찰 일부 / 미니 예시. 사용자가 "작은 단서/예시/설명" 선택 가능. 기본은 2단계

**에피소드 원인 (P0-6)**: 기본값 `undetermined`. `user_misconception` 확정 조건 전부 충족 시에만: 질문 valid / claim·evidence 유효 / evidence가 learner model을 실제 배제 / 용어 문제 아님 / 선행 개념 결손 아님 / 사용자 모델이 명료. 그 외 `question_defect | ai_error | terminology | weak_counterexample | prerequisite_gap`. `prerequisite_gap`이면 downstream 개념 일시 정지, 선행 개념 복구 — 미등록 선행 개념은 간이 등록(claim만, verify는 실행 가능할 때만), 복구 질문은 **burden에만 계산**, 복구 깊이 상한 1(초과 시 downstream=deferred)(A5).

**탈출**: 오개념 확정 에피소드 3회 → allowed는 **explanation(escape) 또는 skip**뿐 — 강제 설명이 아니라 사용자가 설명을 받을지 개념을 미룰지 선택한다(D38, 전이표 §3과 일치). **설명 요청(정상, 이유 선택)**: 즉시 explanation, `support_events`에 기록, end_reason은 완료 여부로. **이의 제기**: question_quality 재평가 또는 investigation. **건너뛰기**: deferred. **clarify**: 2회째는 재표현이 아니라 **양식 전환**(추상→사례, 코드→실행 추적, 긴 문장→변수 하나씩, 전문어→일상어). 3회면 discard.

**피드백 시점 (P0-5, P6)**: 정답 사전 누설 금지 ≠ 피드백 금지.
- 답변 직후: 정오 표시 없이 learner model 반영 ("네 설명은 재시도 자체가 중복을 만든다는 것이지?")
- recheck 완료 후: 확인된 실행 결과·근거 명시 ("확인된 결과에서는 같은 키의 두 번째 요청이 새 결제를 만들지 않았다. 처음 설명은 이 사례와 맞지 않는다")
- 개념 종료: 확인된 설명 / 적용 범위 / 남은 불확실성
- explore 종료: **합의된 부분 / 갈리는 부분 / 판단에 필요한 근거** 세 문장 = `open`의 산출물

### 4.4 AI 오류 처리
investigation 트리거·처리는 v3와 동일하되 두 가지 수정:
- **defect 복권 범위 (P0-10)**: 사용자 첫 답 전체가 aligned가 아니라, **실행으로 확인된 구체 명제만** `vindicated_propositions`에 기록. 수정 전 예측 / 결함 원인 이해 / 수정 후 예측 / 수정 후 경계·전이는 각각 사다리로 확인.
- retract·evidence retract·defect → taint, invalidate-dependents, restore(재판정). `pushed_by_ai`는 신호.

### 4.5 통합·종결
- **integration 진입 (P0-11)**: 최소 한 개념 `demonstrated` ∧ **그 개념이 아닌 다른 개념**에서 why 포함 핵심 단계 pass. demonstrated는 정의상 why pass를 포함하므로, 한 개념만으로는 진입 조건을 충족할 수 없다(서로 다른 두 개념이 필요). 둘 다 약한 partial이면 통합 대신 "관계 한 문장" 선택적 종결 질문. integration은 개별 단계를 보충하지 않는다.
- reexplain: open 재시행, 두 답 나란히.
- 요약 보고 필수 항목: 첫 답 핵심 / 사용한 지원 수준 / 최종 독립 수행 증거 / 남은 경계·불확실성 / 확신도 변화 / 재확인 시점 / incidents·vindicated / `concept` 한계. `learning_outcome`만 크게 띄우지 않는다.

### 4.6 관통 규칙
- 한 화면 한 질문. 정답 사전 누설 금지. 부분 정답 보존은 **모든 단계**(P7).
- **칭찬은 정답 자체보다 근거 제시, 불확실성 표현, 오류 탐지, 질문 제기, 모델 수정에.**
- 이중 카운트(P8): 판정 문항 12~18 목표, 24 경고, 30 비상 상한(정정·안전 종료는 항상 허용). **부담 문항**(폐기·clarify·재질문 포함)이 판정 문항의 1.5배를 넘거나 30을 넘으면 "계속 / 요약 후 종료 / 설명 전환" 선택 제시. 부담 문항의 정의는 **"응답을 요구하는 화면"**(A7) — 비질문 안내 화면은 세지 않는다.
- TUI는 질문 헤더에 `[개념명 · 단계]`를 상시 표시하고, transfer 인터리빙 등 개념 전환 시 비질문 안내 한 줄을 보인다(burden 미계산)(A7).

---

## 5. 아키텍처
v3와 동일. TUI 고정(사용자가 별도 터미널에서 `squiz ui`), IPC 내부 선택(HTTP 후보), 파일이 원천, 이벤트 로그+스냅샷, 응답 원자성, waiter exit 0/2/3/4/5/6/7/8, §5.2 루프는 **0C 스파이크 후 확정**. TUI 동작에 **힌트 등급 선택**과 "계속/요약 종료/설명 전환" 선택 추가.

---

## 6. CLI 명세 (v3 대비 변경분)
- `concept add` stdin에 `target_performance, depth, accepted_alternatives, prerequisites` 추가. `claim_type`에 `norm`.
- `judge` stdin에 `model_clarity, partial_credit` 추가. `question_quality`에 `prerequisite_missing, inaccessible`.
- `ask --kind`에 `recombine, retry, clarify_reply, model_clarify, hint --level 1..4` 추가.
- `episode close --cause undetermined|user_misconception|question_defect|ai_error|terminology|weak_counterexample|prerequisite_gap` (stdin: 확정 체크리스트). 미호출 시 undetermined 유지.
- `defect` stdin에 `vindicated_propositions[]` 필수.
- `explanation record` 후 allowed = `own_words`만; own_words 후 allowed = `boundary|transfer`(새 사례)만.
- `status`에 `judged_count / burden_count` 둘 다.
- `close`가 반환하는 보고에 §4.5 필수 항목.
- 응답 형식·불변조건 I1~I8·저장 구조는 **`squiz-core.md`**(v3에서 재구성, A6 완료).

### 6.4 추가: 교수법 불변조건 P1~P8 (강제 수단 명시)
| # | 불변조건 | 강제 수단 |
|---|---|---|
| P1 | 지원받은 답은 독립 수행으로 계산하지 않는다 | **CLI 게이트**: narrow/hint 후 retry 없이는 stage final 갱신 불가, explanation 후 새 사례 없이는 finalize 불가 |
| P2 | 지연 확인 없이 유지력을 주장하지 않는다 | **CLI**: retention은 세션 내 갱신 불가 |
| P3 | 오개념 귀속 전에 질문·근거·용어·선행 개념을 확인한다 | **CLI**: cause 기본값 undetermined, user_misconception은 체크리스트 stdin 필수. 내용은 **AI 판정** |
| P4 | 단계별로 다른 성공 기준 | **guidance + 0B 검증**. AI 판정 |
| P5 | 도움 요청은 결과를 낮추지 않는다 | **CLI**: end_reason에 설명 요청 없음, support_events 분리 |
| P6 | 탐색 후 확인·미결을 명시한다 | **CLI**: recheck 후·개념 종료 시 feedback 이벤트 없이는 다음 개념 불가. 내용은 AI |
| P7 | 부분 정답 보존은 모든 단계 | **CLI**: partial 판정에 partial_credit 필수. 내용은 AI |
| P8 | 보인 모든 문항은 부담에 계산한다 | **CLI**: burden_count |

---

## 7. SKILL.md 초안 (v3에 추가·수정되는 자세)
```markdown
- 지원받아 답한 것과 스스로 보여준 것은 다르다. 단서·설명 뒤에는 반드시 단서 없는 새 사례를 준다.
- 질문에 조건이 빠졌으면 답을 판정하기 전에 내 질문을 고친다. "왜?"를 같은 말로 반복하지 않는다.
- consequence는 사용자 모델이 관찰 가능한 예측을 낼 때만, 그리고 그 예측을 배제하는 검증된 결과가 있을 때만. 답을 암시하는 질문("그런 코드를 왜 넣었을까?")은 leading이다.
- 에피소드 원인은 확인 전까지 미정이다. 질문·근거·용어·선행 개념을 먼저 의심한다.
- 정답을 미리 말하지 않는 것과 피드백을 주지 않는 것은 다르다. recheck가 끝나면 확인된 결과를 근거와 함께 말한다. 개념이 끝나면 확인된 것과 미결을 말한다.
- 한 세션으로 "확실히 안다"고 보고하지 않는다. 다른 날 재확인을 권한다.
- 칭찬은 정답보다 근거 제시, 불확실성 표현, 내 오류 지적, 모델 수정에 한다.
- 설명 요청·모른다는 말은 불이익이 아니다. 모른다는 사용자에게 단서/예시/설명 중 고르게 한다.
```

---

## 8. 결정 로그 (추가분)
| # | 결정 | 이유 |
|---|---|---|
| D25 | scaffold 후 unscaffolded retry 강제, explanation 후 새 사례 필수 | 지원 수행의 성취 오염 방지 (P1) |
| D26 | `solid` 폐기. `session_consistency` + `retention(untested 기본)` | 한 세션은 유지력을 판정 못 함 |
| D27 | 단계별 rubric, why에서 evidence≠mechanism, `claim_type=norm` | 단계가 측정하는 것이 다름 |
| D28 | cause 기본값 undetermined, prerequisite_gap 추가 | 인식론적 겸손과 일관 |
| D29 | consequence 조건은 model_clarity + 검증 반례. 확신도는 I6에만 | 확신도는 모델 구조의 대리가 아님 |
| D30 | 피드백 시점 3단계 정의 | 숨은 정답 맞히기 방지 |
| D31 | 설명 요청은 support_event, end_reason 아님. 이유 선택 | 도움 요청 불이익 없음 |
| D32 | 판정/부담 이중 카운트, 기본 개념 2~3 | AI 오류 피로 전가 방지 |
| D33 | defect 복권은 명제 단위 | 결함 발견 ≠ 개념 이해 |
| D34 | integration 진입: demonstrated 1 + **다른 개념** why pass 1 | 통합은 복구 장치가 아님. 한 개념만으로 충족 불가(v4.1 명시) |
| D35 | transfer는 다음 개념 뒤로 지연 | 패턴 추종 방지 |
| D36 | P1~P8을 강제 수단별로 구분(게이트 / guidance+0B) | CLI가 강제 못 하는 것을 정직하게 표기 |
| D37 | contested contradicted+sure → 근거 조사, 오개념 카운트 금지; author divergent → investigation 우선 | v3 미결 확정 |
| D38 | 오개념 3회 탈출 allowed = {explanation(escape), skip} | 사용자 자율성(설계 정체성: skip은 정상 경로). v4의 "explanation만"과 전이표 불일치를 skip 허용으로 통일 |
| D39 | 0A A1~A7 확정: 단일 개념 transfer=reexplain 직전+한계 명시 / retry ≤2 / `fail` 폐기 / explanation 후 사다리 재개 / 선행 복구 간이 등록+burden만+깊이 1 / 핵심 규약 v4 재구성(`squiz-core.md`) / TUI 헤더 개념·단계 표시, 부담="응답 요구 화면" | 각 근거는 `phase0a-decisions.md`. P1·P5·P8과 일관 |
| D40 | 구현: Go, bubbletea TUI, 파일 기반 IPC(원자적 rename), goreleaser+GitHub Releases 배포 | 사용자 지정(Go, GitHub 표준 배포). 파일 기반 IPC는 "파일이 원천"과 일치, 0C 스파이크 대체 |

---

## 9. 예시 궤적 (v4 규칙 적용, `ai` 모드)
```
ask:predict     "같은 요청 키로 동일 결제 요청이 두 번 들어오면, 결제 레코드 수와 두 번째 응답은?"
answer          "레코드 2개, 둘 다 성공 응답" / sure
judge           contradicted / convention / explicit_prediction / valid   → I6 recheck
verify --recheck  (e-4: 같은 키 2회 → 레코드 1, 2번째 캐시 응답. excludes "두 번 결제")
feedback(recheck) "확인된 실행에서는 레코드가 1개, 두 번째는 첫 응답과 동일했다. 네 예측과 갈린다."
ask:consequence "네 모델에서 실행 결과와 갈리는 가정은 어느 것일까?"           (에피소드 1)
answer          "서버가 두 요청을 구분 못 한다는 가정" / unsure
judge           partial / mechanism / vague   partial_credit="구분 여부가 핵심이라는 점"
ask:narrow      "서버가 두 요청이 같다는 걸 알 수 있는 정보가 요청에 있어?"
answer          "헤더의 키" / unsure
judge           aligned / mechanism
ask:recombine   "그 키로 서버가 하는 일을 처음 질문에 다시 넣어 설명하면?"
answer          "같은 키면 이전 결과를 돌려주니 레코드 1, 응답 동일" / sure
ask:retry(predict, 새 사례) "첫 요청이 서버에 도달도 못 했으면 두 번째는?"
answer          "키가 처음이니 정상 처리, 레코드 1" / sure
judge           aligned / mechanism / valid   → predict final=pass (unscaffolded)
episode close   cause=user_misconception (체크리스트 충족)
ask:why         "왜 키 하나로 side effect가 한 번만 일어나지? 순서대로"
… boundary … → 개념 B로 이동 → B boundary 후 A의 transfer:
ask:transfer    "이메일 발송에 옮기면, 결제의 어떤 요소가 어디에 대응하고 대응 안 되는 건?"
answer          "키↔메시지 ID, side effect↔발송, 결과 재사용↔이미 보냈다는 응답. 대응 안 되는 건 금액 같은 본문 변경" / sure
judge           aligned / mechanism / valid
feedback(개념 종료) "확인된 것: 키 매칭으로 side effect 1회. 범위: 같은 키+같은 본문. 미결: 키 만료 정책."
finalize        demonstrated / guided / consistent / retention untested / recheck_recommended
```

---

## 10. 인수 테스트 (v3 + 교수법 추가)
**교수법 필수**: own_words만으로 demonstrated 불가 / hint·narrow 뒤 독립 재시도 존재 / AI 오류가 user_misconception으로 남지 않음 / 무효 질문 답 미사용 / 모든 단계에서 partial_credit 명시 / contested는 강제 결론 없이 open / 설명 요청이 실패·중단으로 보고되지 않음 / transfer가 구조 대응 요구 / prerequisite_gap이 오개념으로 안 셈 / 개념 종료 시 확인·미결 명시 / leading 연속 없음 / 부담 카운트 관리.
**0B 관찰 지표**: 독립 transfer까지 질문 수 / 오귀속 사례 수 / AI 질문 결함 수 / leading 문항 수 / 설명 후 반복과 적용 구분 여부 / 이의 제기 후 방어적 대화 여부 / 보고가 처음부터 앎과 지원 후 도달을 구분하는가 / 종료 시 사용자가 확실·불확실을 말할 수 있는가.
**0B 사용자 유형 3종**: 지식은 있으나 표현이 서툰 / 없지만 솔직히 모른다는 / 강한 오개념+높은 확신. 현재 설계는 셋째에 강하고 첫째·둘째에서 과잉 심문·과잉 scaffold 위험.

---

## 11. 미결
~~구현 언어 / TUI 프레임워크 / IPC~~(D40으로 확정: Go / bubbletea / 파일 기반) / 타임아웃 기본값 / author 모드 동거 여부(0B 후) / `concept suggest` / 자동 발동 여부. **UI는 TUI 고정.** 0A 항목은 전부 확정됨(`phase0a-decisions.md`).

## 12. 다음 작업
Phase 0 세 트랙 병렬(0A 명세·0B 교수법·0C 스파이크) → Phase 1 CLI+TUI 동시 → Phase 2(지연 확인, 재생, 질문 품질 자동, 접근성) → Phase 3(발동 품질). 0A에는 §8 D25~D38 반영한 전이표 확정과 P1~P8 게이트 정의, 그리고 **`phase0a-decisions.md`의 A1~A7 결정**이 포함된다. 0B는 §10 지표와 사용자 유형 3종으로.

## 13. 리뷰 반영 이력
v1→v2 인식론 / v2→v3 상태 기계·복구·TUI / v3→v4 교수법: 리뷰 P0-1~P0-11 전부 수용. 조정: P0-7은 `model_clarity` 필드로 구조화. P1~P8은 강제 수단별 구분(D36). / **v4→v4.1 교차 검증**: 설계-전이표 불일치 4건 수정(integration 조건, 오개념 탈출 skip, why S=none, 24 경고), 전이 공백 5건을 0A 결정 항목으로 격리(`phase0a-decisions.md`).

## 14. 등급표 (추가분)
| 항목 | 등급 |
|---|---|
| scaffold 페이딩 게이트(P1) | 필수 |
| retention 세션 내 갱신 금지(P2) | 필수 |
| cause undetermined + 체크리스트(P3) | 필수 |
| support_events 분리(P5), burden_count(P8), partial_credit 필수(P7) | 필수 |
| feedback 이벤트 게이트(P6) | 필수 |
| 단계별 rubric(P4) | guidance + 0B 검증 |
| 힌트 등급 사용자 선택 | 권장(Phase 1 TUI에 포함 가능) |
| transfer 지연 | 필수(전이표) |
| 지연 확인 세션 | Phase 2 |
