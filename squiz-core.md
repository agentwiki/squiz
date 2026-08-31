# squiz 핵심 규약 — 불변조건·응답 승인·waiter·저장 구조 (v4.1 재구성)

> **주의**: 이 문서는 v3 원문 없이 v4 문서의 인용(I1, I2, I6, I7 등)에서 **역산·재구성**한 것이다(0A A6, 사용자 승인). v3 원문이 확보되면 대조 후 번호·정의를 조정한다. 재구성 근거는 각 항목에 표기.
> 구현 확정 사항(언어·TUI·IPC)도 이 문서가 원천이다.

## 1. 인식론 불변조건 I1~I8

CLI(상태 기계)가 강제한다. 위반 이벤트는 거부된다.

| # | 불변조건 | 강제 지점 | 재구성 근거 |
|---|---|---|---|
| I1 | **검증되지 않은 반례로 모순을 대면하지 않는다.** consequence·recheck 피드백은 verify=supported인 evidence를 참조해야 한다 | `ask --kind consequence`는 evidence_ref의 verify=supported 필수 | design §3.1 "검증되지 않은 반례로 모순 대면하지 않는다(I1)" |
| I2 | **오염 복구가 최우선이다.** pending_restore가 비어 있지 않으면 allowed={restore}뿐 | reduce의 allowed 계산 | transitions §4 "pending_restore≠∅ → allowed={restore}만 (I2)" |
| I3 | **모든 응답은 등록된 질문에 귀속된다.** 응답은 현재 pending 질문의 qid와 일치해야 승인 | 응답 수신 시 qid 검사 | transitions §7 "qid 일치 필요" |
| I4 | **한 질문에 한 응답(원자성).** 승인된 응답은 수정·중복 불가. 재응답은 새 질문으로만 | 응답 파일 원자적 rename + qid 소진 | design §5 "응답 원자성" |
| I5 | **판정은 현재 버전을 참조한다.** judge는 현재 claim_version·evidence 버전을 기록하며, 참조 버전이 철회되면 그 판정은 taint | judge 이벤트에 claim_version 기록, retract 시 dependents 무효화 | design §2 taint/restore 정의 |
| I6 | **contradicted+sure는 recheck를 강제한다.** 사용자가 확신하며 반대하면 원천(실행/외부 근거)을 다시 본다. 완료 전 consequence 잠금 | judge 결과가 contradicted+sure면 allowed={recheck}} 계열만 | transitions §1, §4 |
| I7 | **무효 질문의 답은 판정하지 않는다.** question_quality ≠ valid면 alignment 등 판정 필드는 기록만 되고 stage·에피소드 갱신에 사용되지 않는다. judged 미계산, burden 계산 | judge 처리 분기 | design §4.3, transitions §5 |
| I8 | **허용된 동작만 수행한다.** 모든 명령은 현재 상태의 allowed 집합 검사를 통과해야 한다. TUI의 선택(힌트 등급, 계속/요약/설명 전환)도 응답으로 승인 | reduce 진입점 | transitions §7 |

## 2. 응답 승인 규칙 (I3·I4·I8 구체화)

1. `ask`는 질문을 등록하고 qid를 발급한다. pending 질문은 항상 최대 1개(한 화면 한 질문).
2. TUI는 pending 질문만 표시한다. 사용자 응답(텍스트+confidence, 또는 구조적 선택)은 qid를 포함해 원자적으로 기록된다.
3. waiter는 응답 승인 시 해당 exit 코드로 종료한다. 승인된 응답의 수정은 불가(I4); 정정은 다음 질문 또는 이의 제기(exit 7)로.
4. 힌트 등급 선택, "작은 단서/예시/설명" 선택, "계속/요약 종료/설명 전환" 선택은 모두 **구조적 응답**이며 동일한 qid 승인 절차를 거친다.
5. 질문이 pending인 동안 새 `ask`는 거부된다(discard/clarify 절차 예외).

## 3. waiter exit 코드

| exit | 뜻 | 근거 |
|---|---|---|
| 0 | 답변 승인됨(정상). stdout에 응답 JSON | 재구성(관례) |
| 2 | clarify 요청("질문이 무슨 뜻?") | transitions §3 |
| 3 | 타임아웃(응답 없음). 질문은 pending 유지 | 재구성 |
| 4 | 세션 중단 요청 → aborted | transitions §0 |
| 5 | 내부 오류(세션 상태 손상·IPC 실패) | 재구성 |
| 6 | 설명 요청 | transitions §3 |
| 7 | 이의 제기 | transitions §3 |
| 8 | skip(개념 건너뛰기) | transitions §3 |

exit 0 외의 코드에서도 stdout에 부가 정보 JSON(선택 이유 등)을 출력할 수 있다.

## 4. 저장 구조 (파일이 원천)

```
.squiz/
  session.json            # 활성 세션 포인터 {id}
  sessions/<id>/
    events.jsonl          # append-only 이벤트 로그 (원천)
    state.json            # 최신 스냅샷 (이벤트 재생으로 항상 재구성 가능)
    q/                    # IPC: 질문/응답 교환
      question.json       # 현재 pending 질문 (원자적 rename으로 게시)
      response.json       # TUI가 원자적 rename으로 게시, CLI가 승인 후 소비
```

- 모든 쓰기는 temp 파일 작성 후 `rename`(원자성).
- `state.json`은 캐시다. 불일치 시 `events.jsonl` 재생이 이긴다.
- 이벤트는 `{seq, ts, type, payload}`. seq는 단조 증가, 재생 시 reduce를 순서대로 적용.

## 5. 구현 확정 (Phase 1)

| 항목 | 결정 | 이유 |
|---|---|---|
| 언어 | **Go** (모듈 `github.com/agentwiki/squiz`) | 사용자 지정. 단일 바이너리, TUI 생태계 |
| TUI | **bubbletea + lipgloss** | Go TUI 표준 |
| IPC | **파일 기반**(위 §4 q/ 디렉터리, 원자적 rename + 폴링) | v3 §5.2 스파이크(0C)를 대체하는 결정. HTTP 서버 없이 "파일이 원천" 원칙과 일치, waiter·TUI가 독립 프로세스로 동작 |
| 배포 | GitHub Releases + goreleaser, `go install github.com/agentwiki/squiz/cmd/squiz@latest` | Go 표준 배포 방식(사용자 지정) |
| CLI 프레임워크 | 표준 라이브러리 서브커맨드(외부 의존 최소) | 상태 기계가 본체, 플래그는 단순 |
