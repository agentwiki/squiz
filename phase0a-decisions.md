# Phase 0A 결정 체크리스트 (v4.1 교차 검증에서 도출)

> `squiz-design.md`(v4.1)와 `squiz-transitions.md`(v4.1)의 교차 검증에서 발견된, **전이 정의가 비어 있어 Phase 1 생성형 테스트를 만들 수 없는** 항목들. 각 항목은 배경 / 선택지 / 권고로 구성된다. 결정되면 두 문서에 반영하고 여기서 체크한다.
> 결정의 판단 기준은 설계의 정체성(§1)과 P1~P8이다. 어떤 결정도 "지원받은 답을 독립 수행으로 계산하지 않는다"(P1)와 "모름·skip은 정상 경로"를 깨서는 안 된다.

## A1. 개념이 하나뿐일 때 transfer 지연 대체 (v4에서 이월)

- **배경**: transfer 지연(D35)은 다음 개념의 boundary를 시간 간격으로 쓴다. 개념이 1개면 "reexplain 직전"으로 미루지만 실질 간격이 없어 패턴 추종 방지 효과가 의문.
- **선택지**: (a) 그대로 reexplain 직전 + 보고에 "간격 없음" 명시 / (b) transfer를 지연 확인 세션(Phase 2)으로 이월, 세션 내 learning_outcome 상한을 partial로 / (c) 세션 내 transfer 생략, retention과 함께 미검증 처리.
- **권고**: (a). 단일 개념 세션에서 transfer까지 박탈하면 세션 가치가 급감한다. 간격 부재는 판정이 아니라 **보고의 한계 명시**로 다루는 것이 §3.2(정직한 한계)와 일관.

## A2. retry 상한 (v4에서 이월)

- **배경**: scaffold ≤3은 강제되지만 retry 실패 → 재scaffold → retry 루프의 총 상한이 없다.
- **선택지**: (a) 에피소드당 retry ≤2 고정 / (b) 상한 없이 burden_count에만 의존.
- **권고**: (a) **retry ≤2 채택**. scaffold ≤3과 결합하면 에피소드 최악 길이가 유한해지고, 소진 시 전이가 명확해진다(→ A3). 초과 시 allowed={explanation(escape), skip}.

## A3. `stage_state=fail` 진입 조건

- **배경**: 전이표 §1이 `{pending, scaffolded, pass, fail}`을 선언하지만 **어떤 전이도 fail을 설정하지 않는다**. finalize의 demonstrated/partial/not_demonstrated 계산이 stage 상태에 직접 의존하므로 정의 필수.
- **선택지**: (a) fail 상태 폐기 — 미통과는 전부 pending 유지, finalize는 pass 개수만 센다 / (b) 에피소드 자원 소진(scaffold 3 + retry 상한) 후에도 미통과면 fail / (c) explanation 후 새 사례 실패 시에만 fail.
- **권고**: (a) **fail 폐기**. not_demonstrated의 의미가 "모른다"가 아니라 "이 세션에서 증거 미확보"(§3.4)이므로, 개별 stage에도 부정적 종국 상태를 두지 않는 것이 일관된다. 소진된 stage는 pending으로 남고 finalize가 pass 개수로 계산한다. 채택 시 전이표 §1의 상태 집합에서 fail 제거.

## A4. explanation 이후 사다리 재개 여부

- **배경**: 전이표 §3은 "새 사례 통과 → finalize: path=after_explanation"인데, explanation이 predict 에피소드 중에 발생하면 why·boundary·transfer는 pending이다. 새 사례 하나 통과 후 즉시 finalize하면 §8 계산상 pass ≤1 → not_demonstrated가 되어 path=after_explanation과 어긋난 인상을 준다.
- **선택지**: (a) 새 사례 통과 후 **사다리 재개** — 남은 단계를 계속 진행, explanation이 다룬 단계만 after_explanation 경로로 표시 / (b) 새 사례 통과 = 개념 종결, learning_outcome은 stage pass 수로 자동 계산(대개 partial).
- **권고**: (a). explanation은 한 단계의 막힘을 푸는 장치이지 개념을 끝내는 장치가 아니다. P5(도움 요청은 결과를 낮추지 않는다)와도 일관 — (b)는 설명 요청이 사실상 개념을 조기 종결시켜 불이익이 된다. 단, 부담 상한(P8) 도달 시에는 (b)로 강등.

## A5. prerequisite_gap 복구 메커니즘

- **배경**: "downstream 정지, 선행 개념 복구"까지만 정의됨. 선행 개념이 세션에 등록되지 않은 경우의 절차가 없다.
- **결정할 것**: ① 미등록 선행 개념을 임시 개념으로 등록하는가(그렇다면 claim/verify를 거치는가, 간이 절차인가) ② 복구 질문은 judged/burden 어느 쪽에 세는가 ③ 복구 실패 시 downstream 개념의 처리(deferred?) ④ 복구 깊이 상한(선행의 선행 무한 후퇴 방지).
- **권고**: 간이 등록(claim만, verify는 실행 가능할 때만) + 복구 질문은 **burden에만 계산**(P8 정신: 보인 것은 세되, 판정 목표 12~18을 오염시키지 않음) + 복구 상한 1단계, 초과 시 downstream=deferred.

## A6. v3 정의의 v4 인라인화 (자기완결성 회복)

- **배경**: v4.1 문서는 I1~I8, 응답 승인 규칙(v3 §6), waiter exit 0/3/5의 의미, 저장 구조, §5.2 루프를 v3 참조로 처리한다. "다른 AI 세션이 곧바로 재개"라는 문서 목표와 모순.
- **할 일**: v3 문서를 확보해 최소한 I1~I8 한 줄 정의, exit 코드 전체 표, 응답 승인 규칙을 v4 문서에 인라인. **v3가 이 저장소에 없으므로 사용자가 제공해야 한다.**
- [ ] v3 원문 확보
- [ ] 인라인 완료

## A7. transfer 인터리빙의 TUI 표현

- **배경**: A의 transfer는 B 진행 중에 끼어든다(B boundary → A transfer → A concept feedback → B 계속). 규칙상 모순은 없으나 "한 화면 한 질문" 하에서 사용자가 맥락 전환을 인지할 수단이 없다.
- **결정할 것**: 질문 화면에 개념 표시(예: 헤더에 개념명·단계)를 넣을지, 전환 시 한 줄 안내를 burden에 세지 않는 시스템 표시로 처리할지.
- **권고**: 헤더에 `[개념명 · 단계]` 상시 표시 + 개념 전환 시 비질문 안내 한 줄(burden 미계산 — 화면이지만 응답을 요구하지 않으므로 P8의 "부담" 정의를 "응답을 요구하는 화면"으로 정밀화).

---

## 참고: v4 → v4.1에서 이미 확정·수정된 것 (재결정 불필요)

| 항목 | 처리 |
|---|---|
| integrate 진입 조건이 개념 하나로 충족 가능했던 문제 | "demonstrated가 아닌 다른 개념에서 why pass" 명시 (D34 보강) |
| 오개념 3회 탈출: 설계 "explanation만" vs 전이표 skip 포함 | skip 허용으로 통일 (D38) |
| why aligned + S=none 행 누락 | 전이표 §1에 추가 (convention 비-norm과 동일 처리) |
| 판정 24 경고 누락 | 전이표 §0에 추가 |
