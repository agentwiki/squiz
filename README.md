# squiz

소크라테스식 이해 점검 도구 — **Claude Code 스킬 + CLI(상태 기계) + TUI**.

사용자가 어떤 대상(AI가 구현한 것, 일반 개념, 사람이 작성한 코드)을 정답 맞추기가 아니라 **문답으로 스스로 이해에 도달**하게 만드는 형성평가형 튜터입니다. 규칙은 CLI가 강제하고, AI는 질문을 등록한 뒤 대기하며, 사용자는 별도 터미널의 TUI에서 답합니다. 진리의 원천은 AI가 아니라 실행 결과(또는 외부 근거)이며, **도움받아 답한 것과 도움 없이 보여준 것은 구조적으로 구분됩니다.**

## 설치

```bash
go install github.com/agentwiki/squiz/cmd/squiz@latest
```

또는 [Releases](https://github.com/agentwiki/squiz/releases)에서 플랫폼별 바이너리를 내려받으세요.

## 사용

두 개의 터미널이 필요합니다.

**터미널 1 (사용자)** — 답변 화면:

```bash
squiz ui
```

**터미널 2 (AI / Claude Code)** — 세션 진행. Claude Code에서는 `.claude/skills/squiz/SKILL.md` 스킬이 전체 절차를 안내합니다. 수동 개요:

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
- [`.claude/skills/squiz/SKILL.md`](.claude/skills/squiz/SKILL.md) — Claude Code 스킬 (튜터 프로토콜)

## 개발

```bash
go test ./internal/...   # 전이표 기반 생성형 테스트 + 불변조건 테스트 + 재생 결정성
go test ./e2e/ -v        # E2E 사용자 시나리오: 모의 AI 튜터 + 모의 사용자가 실제 바이너리를 구동
go build ./cmd/squiz
```

E2E 테스트는 시나리오별 자연어 전개와 캡처된 입출력 증거를 마크다운 리포트로 남기며, CI가 이를 PR 코멘트로 게시해 리뷰어가 수용 기준(Acceptance Criteria) 관점에서 검증할 수 있게 합니다.

릴리스는 태그 푸시(`v*`)로 GitHub Actions + goreleaser가 수행합니다.
