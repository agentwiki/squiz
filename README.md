# squiz

**"이해했다"는 느낌을 검증하는 도구입니다.**

AI가 코드를 짜 주면 일이 빨리 끝나지만, 그 코드를 내가 정말 이해했는지는 알 수 없습니다. squiz는 AI(Claude Code)가 소크라테스식 문답으로 질문을 던지고, 여러분이 직접 답하면서 스스로 이해에 도달하게 만드는 도구입니다. 정답 맞추기 시험이 아닙니다 — 예측하고, 이유를 설명하고, 어디까지 통하는지 따져 보는 과정을 통해 "안다고 착각한 부분"을 드러내는 것이 목적입니다.

점검 대상은 세 가지 중 하나입니다:

- **AI가 방금 구현해 준 코드** — 내가 이걸 설명할 수 있나?
- **일반 개념** — 예: "멱등성이 뭔지 정말 아나?"
- **사람이 작성한 코드** — 내 코드를 리뷰어 관점에서, 또는 남의 코드를 읽는 관점에서

## 어떻게 동작하나

짧게 요약하면:

- **질문에는 순서가 있습니다.** 결과를 예측하게 하고 → 왜 그런지 설명하게 하고 → 어떤 조건에서 깨지는지 묻고 → 다른 상황에 적용하게 합니다.
- **AI의 말이 정답 기준이 아닙니다.** 기준은 실제 실행 결과(또는 문서 같은 외부 근거)입니다. 여러분이 확신을 갖고 AI와 다르게 답하면, AI는 자기 주장을 다시 검증해야 하고, 틀렸으면 철회합니다.
- **단서를 받아 맞힌 것과 스스로 맞힌 것은 구분됩니다.** 힌트나 설명을 받은 뒤에는 반드시 단서 없는 새 문제로 다시 확인합니다.
- **이 규칙들은 AI의 선의가 아니라 프로그램이 강제합니다.** AI가 절차를 건너뛰려 하면 프로그램이 그 명령을 거부합니다.

원리가 더 궁금하면 [동작 원리](docs/how-it-works.md)를 읽어 보세요. 예비 지식 없이 읽을 수 있게 썼습니다.

## 설치

### Claude Code 플러그인 (권장)

이 저장소는 Claude Code 플러그인 마켓플레이스를 겸합니다. Claude Code 안에서:

```
/plugin marketplace add agentwiki/squiz
/plugin install squiz@squiz
```

이후 "squiz로 이해 점검해줘"라고 요청하면 세션이 시작됩니다. `squiz` 실행 파일이 없으면 GitHub 릴리스에서 자동으로 설치됩니다(`~/.local/bin`).

### 직접 설치

```bash
go install github.com/agentwiki/squiz/cmd/squiz@latest
```

또는 [Releases](https://github.com/agentwiki/squiz/releases)에서 내려받거나, 설치 스크립트를 사용하세요:

```bash
curl -fsSL https://raw.githubusercontent.com/agentwiki/squiz/main/plugins/squiz/scripts/install.sh | sh
```

## 사용법

터미널 두 개를 씁니다. AI와 여러분이 서로 다른 창에 있어야, AI가 대화를 매끄럽게 만들려고 답을 흘리거나 절차를 건너뛰는 일을 막을 수 있기 때문입니다.

**터미널 1 — 여러분(답변 화면):**

```bash
squiz ui
```

**터미널 2 — AI(Claude Code):** "squiz로 이해 점검해줘"라고 요청하면 AI가 알아서 세션을 준비하고 질문을 등록합니다. 질문은 터미널 1의 화면에 하나씩 나타나고, 여러분이 답하면 AI가 다음 질문을 이어갑니다.

답변 화면에서는 답을 입력하는 것 외에도 언제든 할 수 있는 일이 있습니다: **모른다고 말하기, 설명 요청하기, 질문에 이의 제기하기, 이 개념 건너뛰기, 세션 중단하기.** 이 중 어느 것도 불이익이 아닙니다 — 전부 정상적인 진행 경로입니다.

세션이 끝나면 AI가 보고를 줍니다: 스스로 보여준 것, 단서를 받아 도달한 것, 아직 확인되지 않은 것을 구분해서요.

## 문서

처음 오신 분은 위에서 아래 순서로 읽으면 됩니다. 아래로 갈수록 구현 세부사항입니다 — 기여하거나 내부를 고칠 때만 필요합니다.

| 문서 | 대상 | 내용 |
|---|---|---|
| 이 README | 모든 사람 | 무엇인지, 설치, 사용법 |
| [docs/how-it-works.md](docs/how-it-works.md) | 원리가 궁금한 사용자 | 질문의 순서, AI의 말이 기준이 아닌 이유, 도움과 독립 수행의 구분, 세션 보고 읽는 법 |
| [docs/spec/design.md](docs/spec/design.md) | 기여자 | 설계 명세: 판정 구조, 단계별 통과 기준, 규칙과 그 강제 수단 |
| [docs/spec/transitions.md](docs/spec/transitions.md) | 기여자 | 상태 전이표 — 상태 기계 구현과 테스트의 기준 |
| [docs/spec/core.md](docs/spec/core.md) | 기여자 | 불변조건, 응답 승인 규칙, 프로세스 간 통신, 저장 구조 |
| [.claude/skills/squiz/SKILL.md](.claude/skills/squiz/SKILL.md) | AI(튜터) | AI가 세션을 진행할 때 따르는 프로토콜 |

## 개발

```bash
go build ./cmd/squiz
go test ./internal/...   # 상태 기계 테스트
go test ./e2e/ -v        # 모의 AI 튜터 + 모의 사용자가 실제 바이너리를 구동
```

`internal/engine`이 상태 기계이고, 문서가 "프로그램이 강제한다"고 적은 규칙은 전부 여기 있습니다. `internal/cli`와 `internal/tui`는 얇은 껍데기입니다.

규칙을 고칠 때는 [상태 전이표](docs/spec/transitions.md)를 함께 갱신하세요. 단위 테스트가 그 표를 기준으로 전이를 생성해 검증하고, 같은 이벤트 로그를 다시 재생하면 항상 같은 상태가 나오는지도 확인합니다.

명세 문서의 절 번호가 바뀌면 코드 주석이 엉뚱한 곳을 가리키게 됩니다. `python3 scripts/check-doc-refs.py`가 코드의 문서 참조와 문서 간 링크가 실제로 존재하는지 검사하며, CI도 같은 검사를 돌립니다.

E2E 테스트는 시나리오별 전개와 캡처된 입출력을 마크다운 리포트로 남기고, CI가 이를 PR 코멘트로 게시합니다.

`.claude/skills/squiz/SKILL.md`(저장소 로컬)와 `plugins/squiz/skills/squiz/SKILL.md`(플러그인)의 프로토콜 본문은 동일해야 하며 CI가 검사합니다. 한쪽을 고치면 다른 쪽도 함께 고치세요.

릴리스는 태그 푸시(`v*`)로 GitHub Actions + goreleaser가 수행합니다.
