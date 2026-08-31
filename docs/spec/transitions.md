# squiz 상태 전이표

이 문서는 상태 기계가 어떤 상태에서 어떤 사건을 받으면 무엇을 허용하는지를 빠짐없이 적은 표입니다. 구현(`internal/engine`의 `reduce(state, event)`)과 생성형 테스트가 이 표를 기준으로 삼습니다.

**먼저 읽을 것:** [설계 명세](design.md). 이 문서는 그 명세의 규칙이 이미 설명되었다고 보고 전이만 나열합니다. 각 규칙이 **왜** 그런지는 여기 적지 않습니다.

## 표기

표에서 쓰는 약어입니다.

| 약어 | 판정 필드 | 값 |
|---|---|---|
| A | alignment | aligned / contradicted / partial / divergent / unknown / mismatch |
| S | support | mechanism / evidence / convention / none / not_requested |
| M | model_clarity | explicit_prediction / vague / none |
| Q | question_quality | valid / ambiguous / leading / compound / underspecified / off_claim / prerequisite_missing / inaccessible |
| C | confidence | sure / unsure / dontknow |

`I1`~`I8`은 [핵심 규약](core.md)의 인식론적 불변조건, `P1`~`P8`은 [설계 명세](design.md) §15의 교수법 불변조건 번호입니다. `*`는 값이 무엇이든 상관없음을 뜻합니다.

## 1. 세션 단계 전이

세션 단계는 `prep → open → ladder → integrate → close → done` 순으로 진행합니다.

| 현재 단계 | 사건 | 조건 | 다음 | 비고 |
|---|---|---|---|---|
| prep | start | 모든 개념의 verify가 `pending`·`refuted`가 아님. `concept` 모드의 `supported`는 외부 근거 필수 | open | |
| open | 개시 질문 답변 | — | ladder | 판정하지 않음. 기준선으로만 사용 |
| ladder | 개념 A의 boundary 완료 | 다음 개념 있음 | ladder (개념 B) | **A의 transfer는 B의 boundary 이후에 허용** |
| ladder | 개념 B의 boundary 완료 | — | A의 transfer 허용 → 이후 B의 다음 개념 | 마지막 개념의 transfer는 integrate 직전 |
| ladder | 모든 개념 종결 또는 건너뜀 | `demonstrated` 1개 이상 **그리고 그 개념이 아닌 다른 개념**에서 why 통과 1개 이상 | integrate | 서로 다른 두 개념이 필요. 미충족이면 "두 개념의 관계 한 문장" 선택 질문 → close |
| integrate | 통합 판정 2개 이하 | — | close | 통합은 개별 단계를 보충하지 않음 |
| close | 종결 재설명 답변 | — | done | 보고 생성 |
| 모든 단계 | 사용자 중단 (waiter exit 4) | — | done | `end_reason=aborted` |
| 모든 단계 | 판정 문항 24 이상 | — | 변화 없음 | TUI에 경고 표시. 진행은 계속 |
| 모든 단계 | 판정 문항 30 이상 | — | 변화 없음 | 새 학습 질문만 차단 |
| 모든 단계 | 부담 30 초과, 또는 부담 10 이상이면서 판정의 1.5배 초과 | — | 변화 없음 | "계속 / 요약 종료 / 설명 전환" 선택 제시 |

## 2. 사다리 전이 (Q=valid인 경우)

단계 상태는 `pending`, `scaffolded`, `pass` 셋뿐입니다. **실패 상태는 없습니다** — 자원이 소진된 단계는 `pending`으로 남고, 개념 결과는 `pass` 개수로만 계산합니다.

**`pass`는 도움 없는 시도로만 진입합니다.** scaffold를 거친 통과는 `scaffolded`이고, 이어지는 `retry`가 통과해야 `pass`가 됩니다.

| 단계 | A | S / M | 다음 허용 | 상태 갱신 |
|---|---|---|---|---|
| predict | aligned | 단계 기준 충족 | why (또는 why 기준으로 별도 판정 후 boundary) | predict=pass |
| predict | aligned | 기준 미충족 (결과 불명시) | model_clarify | |
| predict | contradicted | C=sure | **원천 재확인** → M=explicit이면 consequence, 아니면 model_clarify | 에피소드 시작 |
| predict | contradicted | M=explicit (C≠sure) | consequence (verify=supported인 근거 필요) | 에피소드 |
| predict | contradicted | M=vague/none | model_clarify → narrow | 에피소드 |
| predict | contradicted | C=dontknow | narrow / hint(등급 선택) / 설명 안내 | 에피소드 |
| predict | partial | * | `partial_credit` 필수 → 틀린 부분만 고립시키는 narrow | 에피소드 |
| predict | divergent | * | investigation 개시 | 진행 정지 |
| predict | unknown | — | narrow / hint 선택 / 설명 안내 | 에피소드 |
| why | aligned | S=mechanism | boundary | why=pass |
| why | aligned | S=evidence | 형식을 바꾼 why 재질문 (과정 순서 / 요소 제거 / 중간 상태) | 근거는 설명이 아님 |
| why | aligned | S=convention, claim_type=norm, 출처·기능 언급 | boundary | why=pass |
| why | aligned | S=convention (norm 아님) | 형식 전환 why 1회 → narrow | consequence 금지 |
| why | aligned | S=none | 형식 전환 why 1회 → narrow | consequence 금지 |
| why | contradicted / partial | predict와 동일 규칙 | narrow의 내용은 "인과의 공백"을 겨냥 | |
| why | divergent | * | investigation | |
| why | mismatch (author 모드의 의도) | * | 중립적 인지 질문 → finding | why=pass (의도 기록, 현재·당시 의도 구분) |
| boundary | aligned | 조건 + 이유 | 다음 개념으로 | boundary=pass |
| boundary | aligned | 조건만 | boundary 재질문 1회 ("왜 그 조건에서 깨지죠?") | |
| boundary | contradicted / partial | predict와 동일 규칙 | narrow의 내용은 "과잉 일반화"를 겨냥 | |
| transfer | aligned | 구조 대응 + 비대응 요소 + 결과 | 개념 종결 가능 | transfer=pass |
| transfer | aligned | 결과만 | transfer 재질문 1회 ("어떤 요소가 어디에 대응하죠?") | |
| transfer | contradicted / partial / unknown | why 복귀 이력 없음 | why로 복귀(추상 구조 재확인) → 새 사례로 재transfer | session_consistency=inconsistent |
| transfer | contradicted | why 복귀 이력 1회 | narrow 1회 → retry → 종결 | |
| transfer | 원인이 선행 개념 | — | 에피소드 원인 `prerequisite_gap` → 후속 개념 정지, 선행 복구 | 오개념 아님. 미등록 선행 개념은 간이 등록(claim만), 복구 질문은 **부담에만 계산**, 복구 깊이 상한 1 — 초과 시 후속 개념 `deferred` |

## 3. 에피소드와 scaffold 전이

한 에피소드 안에서 scaffold 질문은 **최대 3개**입니다. **`recombine`과 `retry`는 scaffold 수에 포함되지 않습니다** — 도움이 아니라 페이딩 단계이기 때문입니다.

| 질문 종류 | 결과 | 다음 (필수) |
|---|---|---|
| model_clarify | M=explicit 획득 | consequence (검증된 근거 필요) 또는 narrow |
| model_clarify | 여전히 vague | narrow |
| consequence | 귀결이 틀림 | 실행 결과 제시(피드백) → "갈리는 가정은?" |
| consequence | 사용자가 모순 인지 | 수정된 모델로 **retry** (같은 단계, 새 시나리오) |
| consequence | divergent | investigation |
| consequence | unknown | narrow |
| narrow | aligned | **recombine 필수** |
| narrow | partial | 한도 내에서 narrow 재분할 |
| narrow | contradicted + sure | 원천 재확인 (I6) |
| narrow | contradicted / unknown | hint (등급 선택) |
| recombine | aligned | **retry 필수** (같은 단계, 도움 없이, 새 표현 또는 새 사례) |
| recombine | 실패 | hint 또는 설명 안내 |
| hint (1~4) | aligned | **retry 필수** |
| hint | 실패 | 다음 등급 힌트(사용자 선택) 또는 설명 개시 |
| retry | aligned + 단계 기준 충족 | 단계=pass, 경로=guided, 에피소드 종료 |
| retry | 실패 | 에피소드 원인 확정 시도 → 한도 내 재scaffold 또는 설명. **에피소드당 retry 2회 상한** — 소진되면 더 이상 `retry`할 수 없고, scaffold 3개 상한까지 함께 소진되었으면 남는 허용 동작은 `explanation`(탈출)과 `skip`뿐 |
| explore | * | explore 2회 이하 → investigation 미결 → **explore 피드백**(합의 / 갈림 / 필요한 근거) → `open` 종결 |

**에피소드 종료 시 원인 기록:** 기본값 `undetermined`. `user_misconception`은 여섯 항목 체크리스트(질문 유효 / claim·evidence 유효 / evidence가 사용자 모델을 배제 / 용어 문제 아님 / 선행 개념 결손 아님 / 사용자 모델 명료)가 **전부 참**일 때만 가능합니다. 그 외에는 `question_defect`, `ai_error`, `terminology`, `weak_counterexample`, `prerequisite_gap` 중 하나입니다.

`session_consistency` 계산에는 `user_misconception`만 셉니다.

## 4. 설명과 사용자 자율성

| 사건 | 다음 |
|---|---|
| `user_misconception` 에피소드 3회 | 허용 동작 = {`explanation`(탈출), `skip`} |
| 설명 요청 (waiter exit 6) | 설명 개시. `support_events`에 `explanation_requested` 추가. 이유 선택 가능 |
| 설명 기록 완료 | 허용 동작 = {`own_words`}뿐 |
| own_words 통과 (재구성 성공) | 허용 동작 = {새 사례 boundary, 새 사례 transfer}뿐 |
| own_words가 설명의 반복 | 형식을 바꾼 적용 문제 1회 (같은 질문 재요청 아님) |
| own_words 불충분 | 추가 설명 또는 `skip` |
| 새 사례 통과 | **사다리 재개** — 남은 단계를 계속 진행. 설명이 다룬 단계만 `after_explanation` 경로로 표시. 모든 단계 소진 후 종결: 경로=after_explanation, 재확인 권장. 단, 부담 상한 도달 시 즉시 종결로 강등 |
| 새 사례 실패 | `learning_outcome=partial` |
| 건너뛰기 (exit 8) | 개념 `deferred` |
| 이의 제기 (exit 7) | 질문 폐기 또는 investigation. 답변은 판정하지 않음 |
| 되묻기 1회째 (exit 2) | `clarify_reply` (재표현) |
| 되묻기 2회째 | `clarify_reply` (**양식 전환**: 추상→사례, 코드→실행 추적, 긴 문장→변수 하나씩, 전문어→일상어) |
| 되묻기 3회째 | 질문 폐기 (`inaccessible`) → 새 질문 |
| dontknow / unknown 답변 | TUI에 "작은 단서 / 예시 / 설명" 선택지를 능동 제시 |

## 5. AI 오류

| 사건 | 다음 | 부수 효과 |
|---|---|---|
| divergent 판정 | investigation 개시 | consequence 잠금 |
| contradicted + sure | 원천 재확인 요구 | consequence 잠금 (I6) |
| 재확인 결과 claim 지지됨 | **확인된 결과를 명시하는 피드백 필수** → 이전 허용 상태로 복귀 | 피드백 없이는 다음 질문 불가 (P6) |
| 재확인 결과 claim이 틀림 | claim 철회 요구 | |
| claim 철회 / evidence 철회 | 버전 +1, taint, 의존 판정 무효화, 복구 대기열 등록 | |
| 복구 대기열이 비어 있지 않음 | 허용 동작 = {`restore`}뿐 (I2) | |
| restore 재판정 | 사다리 재계산 | |
| defect (ai 모드) | stdin의 `vindicated_propositions` 필수 → **해당 명제만** 기록. 단계·개념 통과는 발생하지 않음 | 의존 판정 무효화, 공동 수정 |
| finding (code 모드) | 사건 기록 | author 역할이면 중립적 인지 질문 |
| `contested` 상태 + contradicted + sure | investigation 개시, consequence 계속 잠금. 재확인 대상은 claim과 **양쪽의 외부 근거** | 판별 근거 없으면 미결 → explore → `open`. **오개념으로 세지 않음.** 근거 없이 반대하면 근거를 1회 요청 |
| author 모드 predict divergent | investigation 우선 → 실행 재확인 | 사용자 지지 → claim 철회 / 다르면 관찰을 중립 제시 → 현재·당시 의도 분리 질문 → 검증된 동작이 의도와 다를 때만 `finding` |

## 6. 질문 품질 처리

| Q | 처리 |
|---|---|
| valid | 정상 판정 |
| ambiguous / underspecified | 질문 폐기, 재작성. 답변은 판정에 미사용(I7). 판정 문항에서 제외, **부담 문항에는 포함** |
| compound | 질문 분할 |
| leading | 폐기. 같은 개념에서 2회 발생 시 investigation (AI가 답을 유도하고 있음) |
| off_claim | 폐기, claim 재확인 |
| prerequisite_missing | 답변 판정하지 않음. 선행 개념 복구 질문으로. 에피소드 원인 `prerequisite_gap` |
| inaccessible | 양식을 바꿔 재작성 |

`narrow`와 `leading`의 구분: narrow는 생각할 차원을 줄이되 답을 암시하지 않고, leading은 답을 암시하거나 답이 맞다는 전제를 질문에 심습니다.

## 7. 피드백 게이트

| 시점 | 필수 이벤트 | 없으면 막히는 것 |
|---|---|---|
| 답변 직후 | (선택) 사용자 모델을 되비추는 문장 | — |
| 원천 재확인 완료 | `feedback --kind recheck` (확인된 결과·근거) | 다음 `ask` |
| 개념 종료 | `feedback --kind concept` (확인된 설명 / 적용 범위 / 미결) | `finalize` |
| explore 종료 | `feedback --kind explore` (합의 / 갈림 / 필요한 근거) | `open` 확정 |

## 8. 응답 승인

모든 응답은 현재 대기 중인 질문의 식별자와 일치해야 승인됩니다. 힌트 등급 선택, "작은 단서 / 예시 / 설명" 선택, "계속 / 요약 종료 / 설명 전환" 선택도 모두 구조적 응답이며 같은 승인 절차를 거칩니다. 세부는 [핵심 규약](core.md) §2에 있습니다.

## 9. 개념 결과 자동 계산

| 항목 | 계산 |
|---|---|
| learning_outcome | 4단계 통과(도움 없는 시도) → `demonstrated`. 2~3단계 → `partial`. 1단계 이하 → `not_demonstrated` |
| acquisition_path | scaffold 없음 → `self_reached`. recombine·retry 경유 → `guided`. 설명 경유 → `after_explanation`. 중단 → `stopped` |
| session_consistency | `user_misconception` 에피소드 2회 이상, 또는 why 복귀, 또는 종결 재설명 누락, 또는 오염 판정 기여 → `inconsistent` |
| retention | **항상 `untested`.** 세션 내 갱신 불가 (P2) |
| recheck_recommended | `inconsistent`, 또는 `guided`, 또는 `after_explanation`, 또는 마지막 답이 `unsure` |
| end_reason | `completed` / `skipped` / `aborted` / `limit` / `unresolved`. **설명 요청은 여기 들어가지 않음** (P5) |
