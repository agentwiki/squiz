# squiz

소크라테스식 이해 점검 도구 — **Codex/Claude Code 스킬 + CLI(상태 기계) + TUI**.

사용자가 어떤 대상(AI가 구현한 것, 일반 개념, 사람이 작성한 코드)을 정답 맞추기가 아니라 **문답으로 스스로 이해에 도달**하게 만드는 형성평가형 튜터입니다. 규칙은 CLI가 강제하고, AI는 질문을 등록한 뒤 대기하며, 사용자는 별도 터미널의 TUI에서 답합니다. 진리의 원천은 AI가 아니라 실행 결과(또는 외부 근거)이며, **도움받아 답한 것과 도움 없이 보여준 것은 구조적으로 구분됩니다.**

## 설치

### Codex 플러그인 (권장)

이 저장소는 Codex 호환 플러그인 마켓플레이스입니다. Codex에서:

```bash
codex plugin marketplace add agentwiki/squiz
codex plugin add squiz@squiz
```

이후 새 스레드에서 "squiz로 이해 점검해줘"처럼 요청하면(또는 스킬이 자동 발동하면) 세션이 바로 시작됩니다. **`squiz` 바이너리가 없으면 GitHub 릴리스에서 플랫폼에 맞는 바이너리를 자동으로 설치**하며(`~/.local/bin`), 사용자는 안내에 따라 별도 터미널에서 `squiz ui`를 열어 답하면 됩니다.

### Claude Code 플러그인

이 저장소는 Claude Code 플러그인 마켓플레이스를 겸합니다. Claude Code 안에서:

```
/plugin marketplace add agentwiki/squiz
/plugin install squiz@squiz
```

Codex와 같은 스킬 및 설치 스크립트를 사용합니다.

### 바이너리 직접 설치

```bash
go install github.com/agentwiki/squiz/cmd/squiz@latest
```

또는 [Releases](https://github.com/agentwiki/squiz/releases)에서 플랫폼별 바이너리를 내려받거나, 설치 스크립트를 사용하세요:

```bash
curl -fsSL https://raw.githubusercontent.com/agentwiki/squiz/main/plugins/squiz/scripts/install.sh | sh
```

## 사용

두 개의 터미널이 필요합니다.

**터미널 1 (사용자)** — 답변 화면:

```bash
squiz ui
```

**터미널 2 (AI / Codex 또는 Claude Code)** — 세션 진행. 플러그인의 `skills/squiz/SKILL.md`가 전체 절차를 안내합니다. 수동 개요:

```bash
squiz init --source ai            # ai | concept | code [--role author|reviewer]
squiz concept add --name "..." --claim "..." --claim-type behavior
echo '{"evidence":[{"kind":"execution","text":"...","excludes":"..."}]}' \
  | squiz verify --concept c1 --status supported
squiz start
squiz ask --kind open --text "..."
squiz wait                        # exit 0=답변 2=clarify 4=중단 6=설명요청 7=이의 8=skip
echo '{"alignment":"...","support":"...","model_clarity":"...","question_quality":"valid"}' \
  | squiz judge
squiz status                      # 허용된 다음 동작(allowed) 확인
```

상태는 `./.squiz/`에 저장됩니다(append-only 이벤트 로그가 원천).

### SW 구상 좁히기

구현 전 아이디어도 `concept` 세션의 제품 가설로 다룰 수 있습니다. 화면에는 계속 **질문 하나만** 표시합니다. 여러 질문을 한꺼번에 받으면 답을 빠뜨리기 쉽고 앞 답에 맞춰 다음 질문을 바꿀 수 없기 때문입니다. 대신 시작할 때 진행 속도를 고릅니다.

| 진행 방식 | 동작 | 적합한 경우 |
|---|---|---|
| 빠른 수렴 | 독립적인 결정 축을 짧은 선택형 질문으로 연속 확인하고, 충돌만 후속 질문 | 빠르게 초안이 필요할 때 |
| 균형 | 범위를 크게 바꾸는 답에만 후속 질문 하나 | 기본값 |
| 하나씩 깊게 | 한 축의 성공 기준과 예외까지 정한 뒤 다음 축으로 이동 | 위험하거나 모호한 문제 |

즉 **한 화면 한 질문은 유지하되, 모든 질문을 깊게 파지는 않는 혼합 방식**입니다. 가능한 질문은 텍스트 입력창 대신 TUI의 선택 목록으로 표시합니다. **마우스로 항목을 클릭**하거나 **↑/↓ 또는 j/k + Enter**로 고른 뒤, 별도의 **답변 제출** 버튼으로 확정합니다. 숫자 키는 해당 항목을 선택만 하므로 실수로 즉시 제출되지 않습니다. 마우스 휠로 포커스를 옮길 수도 있습니다. 몇 번의 선택마다 목표·비목표·사용자·핵심 흐름·열린 결정을 요약하고, 계속할지 명세 초안을 만들지 사용자가 고릅니다.

Codex CLI에서는 답을 받을 때마다 채팅 턴을 끝내지 않고, 실행 중인 waiter를 계속 확인한 뒤 판정과 다음 질문까지 같은 턴에서 이어갑니다. 따라서 질문 사이에 사용자가 별도로 "재개" 메시지를 보낼 필요가 없습니다.

자유 서술이 필요한 질문에서는 여러 줄을 적고 **답변 제출** 버튼을 누릅니다. 질문 명확화, 설명 요청, 건너뛰기, 이의 제기, 세션 중단도 화면의 버튼으로 실행할 수 있으며 확신도 역시 별도 항목을 클릭해 고릅니다.

## 핵심 규칙 (CLI가 강제)

- **scaffold 페이딩(P1)**: 단서(narrow/hint) 뒤에는 반드시 단서 없는 재시도(retry). 설명 뒤에는 자기 말 재구성(own_words)과 새 사례. 통과는 unscaffolded 시도로만 기록됩니다.
- **AI 오류는 핵심 경로**: 사용자가 확신하며 반대하면 원천 재확인(recheck)이 강제되고, claim이 틀렸으면 철회·소급 재판정(restore)이 이어집니다.
- **피드백 게이트(P6)**: recheck·개념 종료 후 확인된 결과를 말하지 않으면 다음 진행이 막힙니다.
- **유지력 주장 금지(P2)**: 한 세션으로는 retention이 항상 "untested"입니다.
- **부담 관리(P8)**: 판정 문항과 부담 화면을 이중 계수하고, 한도에서 사용자에게 계속/종료/설명 전환을 묻습니다.

## 문서

- [`squiz-design.md`](squiz-design.md) — 설계 문서 (v4.1)
- [`squiz-transitions.md`](squiz-transitions.md) — 상태 전이표. 생성형 테스트의 원천
- [`squiz-core.md`](squiz-core.md) — 불변조건 I1~I8 · waiter exit 코드 · 저장 구조 · 구현 결정
- [`phase0a-decisions.md`](phase0a-decisions.md) — Phase 0A 결정 기록 (A1~A7, 전부 확정)
- [`.claude/skills/squiz/SKILL.md`](.claude/skills/squiz/SKILL.md) — Claude Code 스킬 (튜터 프로토콜, 저장소 로컬용)
- [`.agents/plugins/marketplace.json`](.agents/plugins/marketplace.json) — Codex 마켓플레이스 카탈로그
- [`plugins/squiz/`](plugins/squiz/) — Codex/Claude Code 플러그인 (공유 스킬 + 바이너리 자동 설치). 프로토콜 본문은 위 스킬과 동일하며 CI가 동기화를 검사

## 개발

```bash
go test ./internal/...   # 전이표 기반 생성형 테스트 + 불변조건 테스트 + 재생 결정성
go test -count=1 ./e2e/ -v # E2E 사용자 시나리오 + 마크다운 리포트 생성(테스트 캐시 비활성화)
go build ./cmd/squiz
```

E2E 테스트는 시나리오별 자연어 전개와 캡처된 입출력 증거를 마크다운 리포트로 남기며, CI가 이를 PR 코멘트로 게시해 리뷰어가 수용 기준(Acceptance Criteria) 관점에서 검증할 수 있게 합니다.

릴리스는 태그 푸시(`v*`)로 GitHub Actions + goreleaser가 수행합니다.
