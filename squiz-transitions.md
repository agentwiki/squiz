# squiz 상태 전이표 (v4.1)

> `squiz-design.md` §6의 `reduce(state, event)`가 구현할 전이. Phase 1 생성형 테스트의 원천.
> 표기: `A/S/M/Q/C` = alignment / support / model_clarity / question_quality / confidence. `I#` 인식론 불변조건, `P#` 교수법 불변조건.
> v4 변경: scaffold 페이딩(P1) 강제, consequence 조건을 M으로, cause 기본값 undetermined, feedback 게이트, transfer 지연, integration 진입 조건, v3 미결 두 건 확정.
> v4.1 변경: integrate 진입 조건에 "서로 다른 두 개념" 명시(D34), why `S=none` 행 추가, 판정 24 경고 행 추가(P8), §9 미결을 `phase0a-decisions.md`로 이관.
> v4.1 확정분(0A A1~A7, D39): 단일 개념 transfer=reexplain 직전+보고 한계 명시(A1), 에피소드당 retry ≤2(A2), `fail` 상태 폐기(A3), explanation 후 사다리 재개(A4), 선행 복구=간이 등록+burden만+깊이 1(A5), TUI 헤더 개념·단계 상시 표시(A7). I1~I8·exit 코드·저장 구조는 `squiz-core.md`.

## 0. 세션 단계

| phase | 이벤트 | 조건 | next | 비고 |
|---|---|---|---|---|
| prep | start | 모든 개념 verify ∉ {pending, refuted}; concept 모드 supported는 external_refs | open | |
| open | open 답변 | — | ladder | 판정 없음. baseline |
| ladder | 개념 A의 boundary 완료 | 다음 개념 있음 | ladder(개념 B) | **A.transfer는 B.boundary 후 allowed** |
| ladder | 개념 B의 boundary 완료 | — | A.transfer 허용 → 이후 B 다음 개념 | 마지막 개념의 transfer는 integration 직전 |
| ladder | 모든 개념 finalize/skip | ≥1 demonstrated ∧ **demonstrated가 아닌 다른 개념**에서 why pass ≥1 | integrate | 서로 다른 두 개념 필요(D34). 아니면 "관계 한 문장" 선택 질문 → close |
| integrate | integrate 판정 ≤2 | — | close | integration은 개별 stage 보충 안 함 |
| close | reexplain 답변 | — | done | 보고(§4.5 필수 항목) |
| * | exit 4 | — | done | aborted |
| * | judged_count ≥ 24 | — | 동일 | TUI에 경고 표시(설계 §4.6). 진행은 계속 |
| * | judged_count ≥ 30 | — | 동일 | 새 학습 질문만 blocked |
| * | burden_count > 1.5×judged ∨ >30 | — | 동일 | TUI에 "계속 / 요약 종료 / 설명 전환" 선택 제시 |

## 1. 사다리 전이 (Q=valid)

`stage_state ∈ {pending, scaffolded, pass}` (A3 확정: `fail` 폐기 — not_demonstrated는 "증거 미확보"이므로 개별 stage에도 부정적 종국 상태를 두지 않는다. 자원 소진된 stage는 pending으로 남고 finalize가 pass 개수로 계산). **`pass`는 unscaffolded 시도로만 진입**(P1). scaffold 후 통과는 `scaffolded`이며 `retry` 통과 시 `pass`.

| stage | A | S / M | next allowed | 갱신 |
|---|---|---|---|---|
| predict | aligned | rubric 충족 | why (또는 why rubric 별도 판정 후 boundary) | predict=pass |
| predict | aligned | rubric 미충족(결과 불명시) | model_clarify | |
| predict | contradicted | C=sure | **recheck** (I6) → M=explicit ? consequence : model_clarify | 에피소드 시작 |
| predict | contradicted | M=explicit (C≠sure) | consequence(verify=supported) | 에피소드 |
| predict | contradicted | M=vague/none | model_clarify → narrow | 에피소드 |
| predict | contradicted | C=dontknow | narrow / hint(등급 선택) / 설명 안내 | 에피소드 |
| predict | partial | * | partial_credit 필수 → narrow(틀린 부분 고립) | 에피소드 |
| predict | divergent | * | investigation open | 정지 |
| predict | unknown | — | narrow / hint 선택 / 설명 안내 | 에피소드 |
| why | aligned | S=mechanism (rubric: 인과) | boundary | why=pass |
| why | aligned | S=evidence | why 재질문(형식 전환: 과정 순서/요소 제거/중간 상태) | evidence는 설명이 아님 |
| why | aligned | S=convention, claim_type=norm, 출처·기능 언급 | boundary | why=pass |
| why | aligned | S=convention (norm 아님) | 형식 전환 why 1회 → narrow | consequence 금지 |
| why | aligned | S=none | 형식 전환 why 1회 → narrow | consequence 금지 (설계 §4.3, v4.1 추가) |
| why | contradicted/partial | (predict 규칙, 단 narrow 내용은 "인과 공백" 지향) | | |
| why | divergent | * | investigation | |
| why | mismatch (author+intent) | * | 중립 awareness 질문 → finding | why=pass(의도 기록, current/original 구분) |
| boundary | aligned | 결과+이유 | (다음 개념으로) | boundary=pass |
| boundary | aligned | 결과만 | boundary 재질문("왜 그 조건에서 깨지지?") 1회 | |
| boundary | contradicted/partial | (narrow 내용은 "과잉 일반화" 지향) | | |
| transfer | aligned | 구조 대응 + 비대응 요소 + 결과 | finalize 가능 | transfer=pass |
| transfer | aligned | 결과만 | transfer 재질문("어떤 요소가 어디에 대응?") 1회 | |
| transfer | contradicted/partial/unknown | why_returns=0 | why 복귀(추상 구조 재확인) → 재transfer(새 사례) | session_consistency=inconsistent |
| transfer | contradicted | why_returns=1 | narrow 1회 → retry → finalize | |
| transfer | (원인이 선행 개념) | — | episode cause=prerequisite_gap → downstream 정지, 선행 복구 | 오개념 아님. A5 확정: 미등록 선행 개념은 간이 등록(claim만), 복구 질문은 **burden에만 계산**, 복구 깊이 상한 1 — 초과 시 downstream=deferred |

## 2. 에피소드·scaffold 전이 (P1 핵심)

에피소드 내 ≤3 scaffold 문항. **recombine·retry는 scaffold 문항 수에 포함되지 않는다**(페이딩 단계).

| kind | 결과 | next (필수) |
|---|---|---|
| model_clarify | M=explicit 획득 | consequence(검증됨) or narrow |
| model_clarify | 여전히 vague | narrow |
| consequence | 귀결 틀림 | 실행 결과 제시(feedback) → "갈리는 가정은?" |
| consequence | 모순 인지 | 사용자가 수정한 모델로 **retry(same stage, 새 시나리오)** |
| consequence | divergent | investigation |
| consequence | unknown | narrow |
| narrow | aligned | **recombine 필수** |
| narrow | partial | narrow 재분할(한도 내) |
| narrow | contradicted+sure | recheck (I6) |
| narrow | contradicted/unknown | hint(등급 선택) |
| recombine | aligned | **retry(same stage, unscaffolded, 새 표현/사례) 필수** |
| recombine | 실패 | hint or 설명 안내 |
| hint(1~4) | aligned | **retry 필수** |
| hint | 실패 | 다음 등급 힌트(사용자 선택) or explanation open(escape) |
| retry | aligned + rubric | stage=pass, path=guided, 에피소드 close |
| retry | 실패 | 에피소드 cause 확정 시도 → 한도 내 재scaffold 또는 explanation. A2 확정: **에피소드당 retry ≤2** — 소진 시 allowed={explanation open(escape), skip} |
| explore | * | explore ≤2 → investigation unresolved → **explore feedback(합의/갈림/필요 근거)** → open |

**에피소드 close**: `cause` 기본 `undetermined`. `user_misconception`은 체크리스트(질문 valid / claim·evidence 유효 / evidence가 모델 배제 / 용어 아님 / 선행 아님 / 모델 명료) 전부 참일 때만. 그 외 `question_defect | ai_error | terminology | weak_counterexample | prerequisite_gap`. session_consistency 계산은 user_misconception만 센다.

## 3. 설명·자율성

| 이벤트 | next |
|---|---|
| user_misconception 에피소드=3 | allowed={explanation open(escape), skip} (D38) |
| exit 6 설명 요청 | explanation open(user). support_events+=explanation_requested. 이유 선택 |
| explanation record | allowed={own_words}만 |
| own_words aligned(재구성) | allowed={boundary(새 사례) \| transfer(새 사례)}만 (P1) |
| own_words = 설명 반복 | 형식 바꾼 적용 문제(재질문 아님) 1회 |
| own_words 불충분 | 추가 설명 or skip |
| 새 사례 통과 | A4 확정: **사다리 재개** — 남은 pending 단계를 계속 진행(explanation이 다룬 단계만 after_explanation 경로). 모든 개념 단계 소진 후 finalize: path=after_explanation, recheck_recommended. 단, 부담 상한(P8) 도달 시 즉시 finalize로 강등 |
| 새 사례 실패 | learning_outcome=partial |
| exit 8 skip | deferred |
| exit 7 이의 | question discard or investigation. 판정 없음 |
| exit 2 clarify (1회째) | clarify_reply(재표현) |
| exit 2 (2회째) | clarify_reply(**양식 전환**) |
| exit 2 (3회째) | discard(inaccessible) → 새 질문 |
| dontknow / unknown | TUI에 "작은 단서 / 예시 / 설명" 선택 능동 제시 |

## 4. AI 오류

| 이벤트 | next | 부수 효과 |
|---|---|---|
| divergent | investigation open | consequence 잠금 |
| contradicted+sure | recheck 요구 | consequence 잠금(I6) |
| recheck supported | resolve supported → **feedback(확인된 결과 명시)** 필수 → 이전 allowed | P6 |
| recheck claim 틀림 | claim retract 요구 | |
| retract / evidence retract | version+1, taint, invalidate-dependents, pending_restore | |
| pending_restore≠∅ | allowed={restore}만 (I2) | |
| restore judge | 재판정 → 사다리 재계산 | |
| defect (ai) | stdin `vindicated_propositions` 필수 → 해당 명제만 기록. stage/개념 통과 없음 (P0-10) | invalidate-dependents, 공동 수정 |
| finding (code) | incidents | author면 중립 awareness 질문 |
| **contested + contradicted+sure** | investigation open, consequence 계속 잠금. recheck 대상=claim과 양쪽 외부 근거 | 판별 근거 없으면 unresolved → explore → open. **user_misconception 카운트 금지**. 근거 없이 반대 답이면 근거 1회 요청 |
| **author predict divergent** | investigation 우선 → 실행 recheck | 사용자 지지 → retract / 다름 → 관찰 중립 제시 → 현재·당시 의도 분리 질문 → 동작≠의도일 때만 finding |

## 5. 질문 품질

| Q | 처리 |
|---|---|
| valid | 정상 |
| ambiguous / underspecified | discard, 재작성. 판정 미사용(I7). judged 제외, **burden 포함**(P8) |
| compound | 분할 |
| leading | discard. 같은 개념 2회 → investigation(AI가 답 유도 중) |
| off_claim | discard, claim 재확인 |
| prerequisite_missing | 답 판정 안 함. 선행 개념 복구 질문. cause=prerequisite_gap |
| inaccessible | 양식 전환 재작성 |

narrow vs leading: narrow는 차원을 줄이되 답을 암시하지 않음. leading은 답 암시 또는 답이 맞다는 전제 삽입.

## 6. 피드백 게이트 (P6)

| 시점 | 필수 이벤트 | 없으면 blocked |
|---|---|---|
| 답변 직후 | (선택) learner model 반영 문장 | — |
| recheck 완료 | `feedback --kind recheck` (확인된 결과·근거) | 다음 ask |
| 개념 종료 | `feedback --kind concept` (확인된 설명 / 범위 / 미결) | finalize |
| explore 종료 | `feedback --kind explore` (합의 / 갈림 / 필요 근거) | open 확정 |

## 7. 응답 승인 (I3, I4, I8)
v3 §6 동일. 추가: 힌트 등급 선택·"계속/요약 종료/설명 전환" 선택도 응답으로 승인(qid 일치 필요).

## 8. finalize 자동 계산
- demonstrated: 4 stage pass(unscaffolded). partial: 2~3. not_demonstrated: ≤1.
- path: scaffold 없음→self_reached; recombine/retry 경유→guided; explanation 경유→after_explanation; 중단→stopped.
- session_consistency: user_misconception 에피소드≥2 ∨ why 복귀 ∨ reexplain 누락 ∨ taint 기여 → inconsistent.
- retention: 항상 untested (세션 내 갱신 불가, P2).
- recheck_recommended: inconsistent ∨ guided ∨ after_explanation ∨ 마지막 답 unsure.
- end_reason: completed / skipped / aborted / limit / unresolved. **explanation_requested는 end_reason이 아님**(P5).

## 9. 누락 확인
- [x] v3 §7 항목 전부
- [x] contested contradicted+sure (§4)
- [x] author predict divergent (§4)
- [x] narrow→recombine→retry 강제 (§2)
- [x] explanation→own_words→새 사례 (§3)
- [x] prerequisite_gap (§1, §5)
- [x] transfer 지연 (§0)
- [x] integration 진입 (§0) — v4.1: "서로 다른 두 개념" 명시
- [x] why aligned S=none (§1, v4.1)
- [x] judged 24 경고 (§0, v4.1)
- [x] **A1~A7 전부 확정** (`phase0a-decisions.md` 참조, 2026-08-31) — fail 폐기(A3, §1), 사다리 재개(A4, §3), 선행 복구(A5, §1), retry ≤2(A2, §2), 개념 1개는 reexplain 직전 transfer + 보고 한계 명시(A1, 설계 §4.2), 핵심 규약 재구성(A6, `squiz-core.md`), TUI 헤더 표시(A7, 설계 §4.6)
