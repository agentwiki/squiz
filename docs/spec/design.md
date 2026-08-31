# squiz 설계 명세

이 문서는 squiz가 무엇을 어떻게 판단하는지를 구현 수준으로 서술합니다. 기여자와 구현자를 위한 문서입니다.

**먼저 읽을 것:** [동작 원리](../how-it-works.md). 그 문서는 이 도구가 왜 이런 방식으로 질문하는지를 예비 지식 없이 설명합니다. 이 문서는 그 위에서, 규칙을 정확한 정의와 판정 조건으로 다시 씁니다. 용어는 이 문서 안에서 다시 정의하므로 순서대로 읽으면 됩니다.

이 문서는 판단의 **내용**을 정의합니다. 상태가 어떻게 전이하는지의 전체 표는 [상태 전이표](transitions.md)에, 상태 기계가 강제하는 런타임 계약(불변조건, 프로세스 간 통신, 저장 구조)은 [핵심 규약](core.md)에 있습니다.

---

## 1. 시스템 구성

세 부분으로 나뉩니다.

| 부분 | 실행 주체 | 책임 |
|---|---|---|
| 스킬 | AI (Claude Code) | 질문 문안 작성, 답변 판정, 근거 확보를 위한 실행 |
| CLI (상태 기계) | `squiz` 프로세스 | 절차 강제. 허용되지 않은 동작은 거부 |
| TUI | `squiz ui` — 사용자 터미널 | 질문 표시, 답변·선택 수집 |

책임 경계가 이 설계의 핵심입니다. **AI는 무엇을 물을지와 답이 무엇을 뜻하는지를 정하고, CLI는 지금 무엇을 해도 되는지를 정합니다.** AI가 절차를 벗어난 명령을 보내면 CLI는 `not allowed`로 거부하고 현재 허용된 동작 목록(`allowed`)을 돌려줍니다. 거부는 오류가 아니라 프로토콜입니다.

AI와 사용자가 서로 다른 프로세스에 있는 이유도 같습니다. 한 대화 안에서 문답이 오가면 AI가 흐름을 바꿀 수 있지만, 질문이 CLI를 거쳐 별도 터미널로 나가면 AI는 등록된 절차 밖으로 나갈 수단이 없습니다.

## 2. 용어

이후 서술에서 쓰는 말들입니다.

| 용어 | 뜻 |
|---|---|
| claim | 개념에 대한 AI의 주장. "이 코드는 이렇게 동작한다". 가설이며 버전을 가진다 |
| claim_type | claim의 종류. `behavior`(동작) / `intent`(의도) / `fact`(사실) / `norm`(규약 자체가 학습 목표) |
| evidence | claim을 뒷받침하는 실행 결과 또는 외부 근거. 버전을 가진다 |
| 사다리 (ladder) | 한 개념을 확인하는 네 단계: predict → why → boundary → transfer |
| scaffold | 막혔을 때 주는 도움. `narrow`(범위 축소) / `hint`(1~4단계) / `explanation`(설명) |
| unscaffolded | 도움 없이 수행한 것. 통과는 이것으로만 기록된다 |
| 에피소드 | 비정렬 판정이 난 지점부터 사다리로 복귀할 때까지의 질문 묶음 |
| model_clarity | 사용자 답이 관찰 가능한 예측을 내는 명료한 모델인지 |
| investigation | claim 하나를 대상으로 하는 의심 조사 |
| explore | 어느 쪽이 옳은지 판정할 근거가 없을 때, 결론을 강제하는 대신 양쪽의 근거와 갈리는 지점을 정리하는 질문. `open` 종결로 이어진다 |
| defect | 점검 도중 확인된 **AI 구현·주장의 결함**. `ai` 모드에서 공동 수정 대상 |
| finding | 점검 도중 확인된 **사람이 작성한 코드의 결함**. `code` 모드에서 기록만 하고 고치지 않는다 |
| awareness 질문 | 검증된 동작이 작성자의 의도와 어긋났을 때, 판정하지 않고 그 사실만 중립적으로 알리는 질문 |
| taint | 철회된 버전을 참조했던 판정이 오염된 상태 |
| restore | 오염된 판정을 새 claim 기준으로 다시 매기는 것 |
| vindicated proposition | 실행으로 사용자 말이 맞았다고 확인된 **구체적 명제 하나** |
| 판정 문항 수 (judged) | 이해 확인에 실제로 쓰인 질문 수 |
| 부담 문항 수 (burden) | 사용자에게 응답을 요구한 모든 화면 수 |
| waiter | `squiz wait`. AI가 답변을 기다리는 블로킹 명령 |

## 3. 뿌리 원칙

### 3.1 진리의 원천은 AI가 아니다

위험은 AI가 자주 틀린다는 데 있지 않고 **틀린 확신이 사용자에게 그대로 옮겨붙는다**는 데 있습니다. 그래서 판정의 기준은 AI의 주장이 아니라 원천입니다. 원천은 `ai`·`code` 모드에서 실행 결과, `concept` 모드에서 외부 근거입니다.

여기서 따라나오는 규칙이 둘입니다. 첫째, **검증되지 않은 반례로 사용자를 모순 대면시키지 않습니다.** 사용자의 예측을 배제하려면 그 배제 근거가 실제로 확인된 것이어야 합니다. 둘째, 판정할 수 없으면 강제로 결론내지 않고 `open`으로 남깁니다.

### 3.2 공유된 맹점

사용자의 답과 AI의 주장이 **같은 방식으로 틀렸다면** 시스템은 알아채지 못합니다. 일치하는 답을 의심할 근거가 없기 때문입니다. `concept` 모드에서 특히 그렇습니다. 완화 수단은 세션 시작 전 외부 반례 확보이고, 그래도 남는 한계는 보고에 항상 명시합니다.

### 3.3 지원받은 수행과 독립 수행은 다르다

도움을 받아 도달한 답은 통과로 기록하지 않습니다. 도움 뒤에는 반드시 도움 없는 재시도가 따르고, 통과는 그 재시도로만 기록됩니다. 이것은 권고가 아니라 CLI가 막는 게이트입니다(§10).

### 3.4 수정 불가능한 설계 정체성

다음은 어떤 변경으로도 깨서는 안 되는 항목입니다.

- AI의 claim도 가설이다.
- 사용자가 근거를 들고 버티면 원천을 다시 본다.
- 질문이 잘못되면 사용자의 답을 판정하지 않는다.
- 모름·설명 요청·이의 제기·건너뛰기·미결 종결은 전부 정상 경로다.
- 예측·근거·경계·전이를 **사용자가 생성한 답**으로 확인한다.
- AI의 오류와 사용자의 학습 결과는 별도 차원으로 기록한다.
- 지원받은 답을 독립 수행으로 계산하지 않는다.

## 4. 소스 모드

`squiz init --source ai|concept|code [--role author|reviewer]`

|  | `ai` | `concept` | `code` |
|---|---|---|---|
| 점검 대상 | AI가 방금 구현한 것 | 일반 개념 | 사람이 작성한 코드 |
| 원천 | 실행 | 외부 근거 | 실행 (의도는 작성자) |
| verify 가능 상태 | supported / refuted / unverifiable | supported / contested / unverifiable | 좌동 + intent_guess |
| supported 조건 | 반례 실행 + 오개념 배제 | 외부 URL·인용 + 문헌 반례 1개 이상. 회상만이면 contested | 반례 실행 + 오개념 배제 |
| 반례의 형태 | 실행 입력 | 사고 실험 (준비 단계에서 만든 것만) | 실행 입력 |
| 개시 질문 | "핵심 결정 셋은?" | "한 문단·예시·코드 흐름 중 편한 방식으로" | author: "네가 내린 결정 셋은?" / reviewer: "작성자는 왜 이렇게 했을까?" |
| 결함 발견 시 | `defect` → 공동 수정 | 해당 없음 | `finding` 기록, 수정하지 않음 |

`intent_guess`는 `code` 모드 전용 상태로, claim이 작성자의 **의도**에 대한 것이라 실행으로 검증할 수 없을 때 씁니다.

verify 상태가 이후 진행을 정합니다. (`consequence`는 사용자 모델의 귀결을 검증된 실행 결과와 대면시키는 질문입니다. 정확한 조건은 §7에 있습니다.)

- `supported` — 전체 사다리 + consequence 질문 허용
- `contested` / `unverifiable` / `intent_guess` — 사다리와 explore는 허용, **consequence 금지** (검증되지 않은 근거로 모순 대면 불가)
- `refuted` / `pending` — 세션 시작 차단

**`code --role author`** (자기 코드를 리뷰): predict는 실제 동작(검증 대상), why는 작성자의 의도(권위를 인정하므로 `contradicted` 판정 불가), boundary는 동작과 의도가 갈라지는 지점입니다. 보고에서는 **현재 의도**와 **당시 의도에 대한 회상**을 구분해 적습니다.

## 5. 세션 흐름

세션은 다섯 단계를 지납니다.

```
prep ──start──▶ open ──개시답변──▶ ladder ──▶ integrate ──▶ close ──▶ done
```

### 5.1 prep (준비)

1. 개념을 등록합니다. **기본 2~3개, 최대 5개.** 판정 문항 12~18개가 목표입니다.
2. 개념마다 다음을 함께 등록합니다: claim과 claim_type, **목표 수행**(어디서 쓸 수 있어야 하는가), **필요 깊이**(사용/리뷰/설계/설명), **허용 대안 설명**(문구가 달라도 인정할 모델), 예상 오개념, 반례, 의존 관계, **필요 선행 개념**.
3. verify: 오개념을 배제하는 증거를 붙입니다(`--excludes`). 판정 기준은 문구 일치가 아니라 **핵심 관계·조건·예측의 보존**입니다.
4. 순서는 의존 관계 순으로, 첫 개념은 쉬운 것으로 놓습니다.

### 5.2 open (개시)

먼저 프레임을 제시합니다: 내 주장도 검토 대상이다 / 되물어도 된다 / 설명 요청·이의 제기·건너뛰기가 가능하다 / **"첫 답은 판정하지 않고 난이도와 용어를 맞추는 데만 씁니다."**

개시 질문의 답은 판정하지 않고 기준선으로만 씁니다.

### 5.3 ladder (사다리)

개념 A에 대해 predict → why → boundary를 진행하고, **A의 transfer는 묻지 않은 채** 개념 B로 넘어갑니다. A의 transfer는 B의 boundary가 끝난 뒤에 끼어듭니다.

이렇게 미루는 이유는 직전 답의 문장 구조를 흉내내는 것만으로 전이 질문에 답할 수 있기 때문입니다. 사이에 다른 개념을 끼우면 그 흉내가 통하지 않습니다.

마지막 개념의 transfer는 integrate 직전에 묻습니다. 개념이 하나뿐이면 종결 직전에 묻되, 실질적 간격이 없으므로 **보고에 "지연 간격 없음"을 한계로 명시**합니다.

### 5.4 integrate (통합)

진입 조건: **한 개념 이상이 `demonstrated`이고, 그 개념이 아닌 다른 개념에서 why 단계를 통과**했을 것. `demonstrated`는 네 단계를 전부 도움 없이 통과한 상태를 뜻합니다(정확한 계산은 §14). 정의상 why 통과를 포함하므로, 이 조건은 서로 다른 두 개념을 요구합니다.

둘 다 약한 상태면 통합 대신 "두 개념의 관계를 한 문장으로" 선택적 종결 질문을 냅니다. 통합 질문은 최대 2개입니다. **통합 질문은 개별 단계의 미통과를 보충하지 않습니다.**

### 5.5 close (종결)

개시 질문을 다시 냅니다(`reexplain`). 두 답을 나란히 놓고 보고를 만듭니다. 보고 필수 항목은 §14에 있습니다.

## 6. 사다리 단계별 통과 기준

같은 "정렬됨 + 기제 제시"라도 단계마다 요구하는 증거가 다릅니다.

| 단계 | 통과 증거 | 불충분한 답 |
|---|---|---|
| predict | 조건이 주어진 상황에서 **관찰 가능한 결과**를 명시 | "달라질 것 같다" |
| why | 결과를 낳는 **인과 관계·불변량·규칙** | "실행해보니 그랬다"는 근거일 뿐 설명이 아님. "원래 그렇게 한다"는 claim_type이 `norm`이고 출처·기능까지 말할 때만 충분 |
| boundary | 어떤 조건이 바뀌면 결과가 달라지는지 **와 그 이유** | 결과만 맞힘, 희귀 예외 나열 |
| transfer | 원 사례의 본질 구조 → 새 사례 요소로의 **대응**, 대응되지 않는 표면 요소 식별, 결과 예측 | "둘 다 중복을 막는다" |
| own_words | 들은 설명의 핵심 관계를 자기 말로 재구성 | 설명 문구 반복, 키워드 나열 |

`own_words`는 사다리의 다섯 번째 단계가 아닙니다. 사다리는 네 단계 그대로이고, `own_words`는 사용자가 설명을 들은 뒤에만 끼어드는 확인 절차입니다(§10).

predict 답변이 인과를 포함하면 why를 면제할 수 있지만, **why 기준으로 별도 판정한 뒤에만** 가능합니다.

질문 작성 규칙:

- predict 질문은 초기 조건·변화 변수·관찰 결과·범위를 포함해야 합니다. 조건이 빠졌으면 `underspecified`입니다.
- why 재질문은 "왜?"의 반복이 아니라 **형식 전환**입니다: 과정 순서 / 요소 제거 시 결과 / 중간 상태 / 두 설명 비교.
- boundary 사례는 희귀한 함정이 아니라 **적용 범위를 가르는 조건**이어야 합니다.

## 7. 판정 구조

답변마다 AI가 다음 구조로 판정을 제출합니다(`squiz judge`, stdin JSON).

```json
{
  "alignment": "aligned | contradicted | partial | divergent | unknown | mismatch",
  "support": "mechanism | evidence | convention | none | not_requested",
  "model_clarity": "explicit_prediction | vague | none",
  "question_quality": "valid | ambiguous | leading | compound | underspecified | off_claim | prerequisite_missing | inaccessible",
  "confidence": "sure | unsure | dontknow",
  "partial_credit": "맞은 부분 (alignment=partial이면 필수, 모든 단계에서)",
  "misconception": null,
  "claim_version": 1,
  "evidence_refs": [],
  "rationale": "..."
}
```

각 필드의 뜻:

- **alignment** — 답이 claim과 어떤 관계인가. `divergent`는 사용자가 AI 주장과 다른 동작을 주장하는 경우, `mismatch`는 검증된 동작과 작성자 의도가 어긋난 경우, `unknown`은 모른다는 답입니다.
- **support** — 근거의 종류. why 단계에서 `mechanism`만이 통과 근거이고 `evidence`는 아닙니다(§6).
- **model_clarity** — 답이 관찰 가능한 예측을 내는가. consequence 질문의 진짜 조건입니다.
- **question_quality** — **AI 자신의 질문에 대한 평가**입니다. `valid`가 아니면 사용자의 답은 판정에 쓰이지 않고, 질문을 폐기하고 다시 만들어야 합니다. `prerequisite_missing`은 선행 개념 복구로, `inaccessible`은 표현 양식 전환으로 이어집니다.
- **confidence** — 사용자가 스스로 밝힌 확신도. TUI에서 답과 함께 수집합니다.

### consequence 질문의 조건

consequence는 사용자 모델의 귀결을 실행 결과와 대면시키는 질문입니다. 다음 셋을 모두 만족할 때만 낼 수 있습니다.

1. `model_clarity = explicit_prediction` — 모델이 관찰 가능한 예측을 낸다
2. 그 예측을 배제하는 **검증된** 반례가 있다
3. 진행 중인 investigation이 없다

**확신도는 조건이 아닙니다.** `sure`는 원천 재확인 강제(§11)에만 씁니다.

consequence 질문은 수사적 질문이 아니라 관찰 가능한 예측을 요구해야 합니다. "네 설명대로면 결제 레코드 수와 두 번째 응답은?" → 실행 결과 제시 → "예상과 갈리는 가정은?" 순서입니다. "그런 코드를 왜 넣었을까?"는 답을 암시하므로 `leading`입니다.

### narrow와 leading의 구분

- **narrow** — 생각할 차원을 줄이되 답을 암시하지 않는다.
- **leading** — 답을 암시하거나, 답이 맞다는 전제를 질문에 심는다.

같은 개념에서 `leading`이 두 번 나오면 AI가 답을 유도하고 있다는 신호로 보고 investigation을 엽니다.

## 8. 주요 전이 요약

전체 표는 [상태 전이표](transitions.md)에 있습니다. 자주 쓰이는 것만 옮기면:

| 판정 | 다음 |
|---|---|
| aligned + 단계 기준 충족 | 다음 단계 |
| aligned + convention/none (claim_type이 norm이 아님) | 형식을 바꾼 why 재질문 → narrow. consequence 금지 |
| divergent | investigation → 원천 재확인 |
| contradicted + sure | 원천 재확인 강제 → 이후 model_clarity가 explicit이면 consequence, 아니면 모델 명료화 질문 |
| contradicted + explicit_prediction (unsure 포함) | consequence (검증된 반례 필요) |
| contradicted + vague/none | 모델 명료화 질문 → narrow |
| contradicted + dontknow / unknown | narrow, 또는 사용자가 힌트 등급 선택, 또는 설명 안내 |
| partial (모든 단계) | 맞은 부분을 명시해 인정 → 틀린 부분만 고립시키는 narrow |
| mismatch | 중립적 인지 질문 → finding 기록 |

## 9. 에피소드와 원인 귀속

비정렬 판정이 나면 에피소드가 열립니다. 에피소드 안에서 scaffold 질문은 **최대 3개**, 도움 없는 재시도는 **최대 2회**입니다. 둘 다 소진되면 허용 동작은 `explanation`(설명)과 `skip`(건너뛰기) 둘뿐입니다.

에피소드를 닫을 때 **원인**을 기록합니다. 기본값은 `undetermined`이고, 이것이 이 설계에서 중요한 지점입니다.

`user_misconception`(사용자의 오개념)으로 기록하려면 다음 여섯 가지가 **모두** 참이어야 하며, CLI가 이 체크리스트를 stdin으로 요구합니다.

1. 질문이 유효했다 (`question_valid`)
2. claim과 evidence가 유효하다 (`claim_evidence_valid`)
3. evidence가 사용자의 모델을 실제로 배제한다 (`evidence_excludes_model`)
4. 용어 문제가 아니다 (`not_terminology`)
5. 선행 개념 결손이 아니다 (`not_prerequisite`)
6. 사용자의 모델이 명료했다 (`model_clear`)

하나라도 거짓이면 다른 원인을 씁니다: `question_defect`(질문 결함) / `ai_error`(AI 오류) / `terminology`(용어) / `weak_counterexample`(반례가 약함) / `prerequisite_gap`(선행 개념 결손).

세션 일관성 계산에는 `user_misconception`만 셉니다. 나머지 원인은 사용자의 이해 문제가 아니기 때문입니다.

**`prerequisite_gap` 처리:** 해당 개념에 의존하는 후속 개념을 일시 정지하고 선행 개념을 복구합니다. 선행 개념이 세션에 등록되어 있지 않으면 간이 등록합니다(claim만 등록, verify는 실행 가능할 때만). 복구 질문은 **부담 문항에만 계산하고 판정 문항에는 넣지 않습니다** — 보인 화면은 세되 판정 목표를 오염시키지 않기 위해서입니다. 복구 깊이는 1단계로 제한하며, 선행의 선행까지 필요하면 후속 개념을 `deferred`로 넘깁니다.

오개념으로 확정된 에피소드가 **3회** 쌓이면 허용 동작은 `explanation`과 `skip` 둘로 좁혀집니다. 설명을 강제하는 것이 아니라, 설명을 받을지 개념을 미룰지 사용자가 고릅니다.

## 10. scaffold 페이딩

도움 뒤에 반드시 따라야 하는 사슬이며, CLI가 강제합니다.

- `narrow`가 통과하면 → **`recombine` 필수** (그 조각을 원래 질문에 다시 넣어 전체를 설명) → **`retry` 필수** (같은 단계, 도움 없이, 새 표현 또는 새 사례). 단계 통과는 이 `retry` 결과로만 기록됩니다.
- `hint`가 통과하면 → **`retry` 필수.**
- `explanation` 이후 → `own_words`(자기 말 재구성) → **새 사례**로 boundary 또는 transfer → **사다리 재개.**

`recombine`과 `retry`는 도움이 아니라 페이딩 단계이므로 scaffold 3개 상한에 포함되지 않습니다.

**설명 이후 사다리 재개**가 중요합니다. 설명이 predict 단계에서 발생하면 why·boundary·transfer는 아직 미진행 상태입니다. 새 사례 하나 통과했다고 개념을 종결해 버리면 통과 단계가 1개 이하가 되어 결과가 부당하게 낮아지고, 설명을 요청한 것이 사실상 불이익이 됩니다. 그래서 새 사례를 통과하면 남은 단계를 계속 진행하고, **설명이 다룬 단계만** `after_explanation` 경로로 표시합니다. 다만 부담 상한(§13)에 도달했으면 즉시 종결로 강등합니다.

`own_words` 답이 설명 문구의 반복이면 같은 질문을 반복하지 않고 **형식을 바꾼 적용 문제**를 냅니다.

힌트는 가벼운 순서로 네 등급입니다: ① 주의를 돌릴 단서 ② 사실 하나 ③ 반례 관찰의 일부 ④ 미니 예시. 기본은 2등급이며, 사용자가 "작은 단서 / 예시 / 설명" 중에서 직접 고를 수 있습니다.

## 11. AI 오류 처리

AI 오류는 예외 처리가 아니라 핵심 경로입니다.

| 상황 | 처리 |
|---|---|
| `divergent` 판정 | investigation을 열고 consequence를 잠근다 |
| `contradicted` + `sure` | 원천 재확인(recheck) 강제. 완료 전까지 consequence 잠금 |
| 재확인 결과 claim이 옳음 | 확인된 결과를 명시하는 피드백 필수 → 이전 흐름 복귀 |
| 재확인 결과 claim이 틀림 | claim 철회(retract) |
| 철회 발생 | 버전 증가, 그 버전을 참조한 판정은 taint, 의존 판정 무효화, 복구 대기열에 등록 |
| 복구 대기열이 비어 있지 않음 | 허용 동작은 `restore`뿐. **오염 복구가 최우선** |
| `restore` | 원래 답변을 새 claim 기준으로 다시 판정 → 사다리 재계산 |

**결함 복권의 범위:** 사용자가 AI의 결함을 찾아냈을 때, 기록되는 것은 사용자의 첫 답 전체가 옳았다는 것이 아니라 **실행으로 확인된 구체적 명제 하나**입니다(`vindicated_propositions`). `defect`를 기록할 때 이 명제 목록이 필수입니다. 수정 전 예측, 결함 원인 이해, 수정 후 예측, 수정 후 경계·전이는 각각 사다리로 따로 확인합니다. **결함을 찾은 것과 개념을 이해한 것은 다른 일입니다.**

두 가지 특수 경우:

- **`contested` 상태에서 `contradicted` + `sure`:** investigation을 열고 consequence를 계속 잠급니다. 재확인 대상은 사용자의 답이 아니라 **claim과 양쪽의 외부 근거**입니다. 판별 근거가 없으면 미결로 두고 explore(최대 2회)를 거쳐 `open`으로 종결합니다. **이 경우는 오개념으로 세지 않습니다.** 나중에 새 근거가 나와도 소급해서 오개념으로 기록하지 않고, 새 근거를 제시한 뒤 다시 판단할 기회를 줍니다.
- **`code --role author`에서 predict가 `divergent`:** investigation과 재확인이 먼저입니다. 실행 결과가 사용자의 예측을 지지하면 claim을 철회합니다. 다르면 관찰된 사실을 중립적으로 제시하고("확인된 실행 결과는 X인데 말씀하신 의도 Y와 다릅니다. 의도된 동작인가요, 예상 밖인가요, 근거를 더 봐야 하나요?"), 현재 의도와 당시 의도를 분리해 물은 뒤, 검증된 동작이 의도와 다를 때만 `finding`으로 기록합니다.

## 12. 피드백

"정답을 미리 누설하지 않는 것"과 "피드백을 주지 않는 것"은 다릅니다. 세 지점에서 피드백이 **필수**이며, 없으면 다음 동작이 막힙니다.

| 시점 | 내용 | 없으면 |
|---|---|---|
| 답변 직후 | 정오 표시 없이 사용자 모델을 되비춤 ("네 설명은 재시도 자체가 중복을 만든다는 것이지?") | 선택 사항 |
| 원천 재확인 완료 후 | 확인된 실행 결과·근거를 명시 | 다음 질문 불가 |
| 개념 종료 시 | 확인된 설명 / 적용 범위 / 남은 불확실성 | 개념 종결 불가 |
| explore 종료 시 | 합의된 부분 / 갈리는 부분 / 판단에 필요한 근거 — 이 세 문장이 `open` 종결의 산출물 | `open` 확정 불가 |

칭찬의 대상도 규정합니다: 정답 자체가 아니라 **근거 제시, 불확실성 표현, AI 오류 탐지, 질문 제기, 모델 수정**에 합니다.

## 13. 부담 관리

두 개의 수를 따로 셉니다.

- **판정 문항 수** — 이해 확인에 실제로 쓰인 질문. 목표 12~18개.
- **부담 문항 수** — 사용자에게 **응답을 요구한 모든 화면**. 폐기된 질문, 되물음, 재시도, 선행 개념 복구 질문이 전부 포함됩니다. 응답을 요구하지 않는 안내 화면은 세지 않습니다.

한도:

| 조건 | 처리 |
|---|---|
| 판정 24 이상 | TUI에 경고 표시. 진행은 계속 |
| 판정 30 이상 | 새 학습 질문 차단. 정정과 안전한 종결은 항상 허용 |
| 부담 30 초과, 또는 부담 10 이상이면서 판정의 1.5배 초과 | "계속 / 요약 후 종료 / 설명 전환"을 사용자에게 선택하게 함 |

부담이 판정보다 훨씬 많다는 것은 세션이 학습이 아니라 헛돌고 있다는 뜻입니다. AI의 오류나 나쁜 질문에서 생긴 피로를 사용자에게 전가하지 않기 위한 장치입니다.

TUI는 질문 헤더에 `[개념명 · 단계]`를 상시 표시하고, transfer가 다른 개념 사이에 끼어들 때는 개념 전환을 알리는 한 줄 안내를 보입니다. 이 안내는 응답을 요구하지 않으므로 부담에 계산하지 않습니다.

## 14. 개념 결과 계산

개념이 끝나면 CLI가 결과를 **자동으로 계산합니다. AI가 찍지 않습니다.**

```json
{
  "learning_outcome": "demonstrated | partial | not_demonstrated | open | deferred",
  "acquisition_path": "self_reached | guided | after_explanation | stopped",
  "session_consistency": "consistent | inconsistent",
  "retention": "untested | delayed_demonstrated | delayed_failed",
  "recheck_recommended": true,
  "end_reason": "completed | skipped | aborted | limit | unresolved",
  "support_events": ["hint", "narrow", "explanation_requested"],
  "incidents": [{ "type": "defect_found | finding | claim_retracted | evidence_retracted | question_discarded", "ref": "..." }],
  "vindicated_propositions": ["..."]
}
```

계산 규칙:

- **learning_outcome** — 네 단계 전부 통과이고 **각 통과가 도움 없는 시도**였으면 `demonstrated`. 2~3단계면 `partial`, 1단계 이하면 `not_demonstrated`. `not_demonstrated`의 뜻은 "모른다"가 아니라 **"이 세션에서 증거를 얻지 못했다"**입니다.
- **acquisition_path** — 도움 없이 도달했으면 `self_reached`, narrow·hint 뒤 도움 없는 재시도로 통과했으면 `guided`, 설명 뒤 새 사례를 통과했으면 `after_explanation`.
- **session_consistency** — 다음 중 하나면 `inconsistent`: `user_misconception`으로 **확정된** 에피소드 2회 이상 / transfer 실패 후 why로 복귀 / 종결 재설명 누락 / 오염된 판정에 기여.
- **retention** — 세션 내 판정으로는 **항상 `untested`**입니다. 지연 확인 세션 이후에만 갱신되며, 세션 안에서는 갱신 자체가 불가능합니다.
- **recheck_recommended** — `inconsistent`이거나, 경로가 `guided`/`after_explanation`이거나, 마지막 답이 `unsure`일 때. 다음에 무엇을 먼저 볼지 정하는 우선순위 정보입니다.
- **end_reason** — 완료 여부만 담습니다. **설명 요청은 여기 들어가지 않고 `support_events`로 갑니다.** 도움을 요청한 것이 결과를 낮춰서는 안 되기 때문입니다.

단계 상태는 `pending / scaffolded / pass` 셋뿐이고 **실패 상태는 없습니다.** 자원이 소진된 단계는 `pending`으로 남고, 결과는 통과 개수로만 계산합니다. `not_demonstrated`의 의미가 "증거 미확보"인 것과 일관됩니다.

**보고에서 쓰는 표현:** 내부 용어를 그대로 노출하지 않습니다.

| 내부 값 | 보고 표현 |
|---|---|
| demonstrated | "이 세션에서 독립적으로 설명하고 적용함" |
| guided | "단서를 활용해 재구성함" |
| after_explanation | "설명을 들은 뒤 새 사례에 적용함" |
| not_demonstrated | "이번 대화에서는 아직 확인되지 않음" |
| retention: untested | "다른 날 짧은 재확인을 권장" |
| misconception | "초기 모델" / "재검토한 가정" / "범위를 수정한 설명" |

**세션 보고 필수 항목:** 첫 답의 핵심 / 사용한 지원 수준 / 최종 독립 수행의 증거 / 남은 경계와 불확실성 / 확신도의 변화 / 재확인 권장 시점 / 기록된 사건과 복권된 명제 / **이 세션의 한계**. 마지막 항목에는 공유된 맹점(§3.2)과, 개념이 하나뿐이라 transfer에 지연 간격이 없었던 경우(§5.3)가 들어갑니다. `learning_outcome`만 크게 띄우지 않습니다.

## 15. 교수법 불변조건

여기까지의 규칙 중 깨져서는 안 되는 여덟 가지를 강제 수단과 함께 정리합니다. **어떤 것이 프로그램으로 강제되고 어떤 것이 AI의 판단에 달려 있는지 구분하는 것이 이 표의 목적입니다.**

| # | 불변조건 | 강제 수단 |
|---|---|---|
| P1 | 지원받은 답을 독립 수행으로 계산하지 않는다 | **CLI 게이트** — narrow·hint 뒤 재시도 없이는 단계 통과 갱신 불가, 설명 뒤 새 사례 없이는 종결 불가 |
| P2 | 지연 확인 없이 유지력을 주장하지 않는다 | **CLI** — `retention`은 세션 내 갱신 불가 |
| P3 | 오개념으로 귀속하기 전에 질문·근거·용어·선행 개념을 확인한다 | **CLI** — 원인 기본값 `undetermined`, `user_misconception`은 체크리스트 필수. 체크리스트 내용의 판단은 AI |
| P4 | 단계마다 다른 성공 기준을 적용한다 | **문서 지침 + 관찰**. AI 판단 |
| P5 | 도움 요청은 결과를 낮추지 않는다 | **CLI** — `end_reason`에 설명 요청이 없고 `support_events`로 분리 |
| P6 | 탐색 후에는 확인된 것과 미결을 명시한다 | **CLI** — 재확인 후·개념 종료 시 피드백 이벤트 없이는 진행 불가. 내용은 AI |
| P7 | 부분 정답 보존은 모든 단계에서 | **CLI** — `partial` 판정에 `partial_credit` 필수. 내용은 AI |
| P8 | 사용자에게 보인 모든 문항을 부담에 계산한다 | **CLI** — `burden_count` |

이와 별개로, 상태 기계가 강제하는 인식론적 불변조건 여덟 가지(I1~I8)가 [핵심 규약](core.md)에 있습니다.

## 16. CLI 명령

```
세션:    init | concept add | verify | start | status | close | version
질문:    ask | wait | judge | feedback | ui
에피소드: episode close | explanation record | finalize | skip
AI 오류: retract | restore | defect | finding | investigation open|close | objection
```

구조적 입력이 필요한 명령은 stdin으로 JSON을 받습니다.

- `concept add` — `target_performance`, `depth`, `accepted_alternatives`, `prerequisites`, `expected_misconceptions`
- `verify` — `evidence[]` (각각 `kind`, `text`, `excludes`), `concept` 모드에서는 `external_refs[]` 필수
- `judge` — §7의 판정 구조
- `episode close --cause <원인>` — `user_misconception`일 때 여섯 항목 체크리스트
- `defect` — `vindicated_propositions[]` 필수
- `restore` — 재판정 내용

`ask --kind`가 받는 질문 종류: `open`, `predict`, `why`, `boundary`, `transfer`, `consequence`, `narrow`, `recombine`, `retry`, `model_clarify`, `hint --level 1..4`, `clarify_reply`, `explore`, `own_words`, `integrate`, `reexplain`, `awareness`, `prereq_recovery`, `session_limit_choice`.

상태 확인은 `squiz status`이며, 현재 허용된 동작(`allowed`)과 두 계수(`judged_count`, `burden_count`)를 함께 돌려줍니다.

## 17. 예시 궤적

`ai` 모드에서 멱등성 개념을 점검하는 흐름입니다.

```
ask:predict     "같은 요청 키로 동일 결제 요청이 두 번 들어오면, 결제 레코드 수와 두 번째 응답은?"
answer          "레코드 2개, 둘 다 성공 응답" / sure
judge           contradicted / convention / explicit_prediction / valid   → 원천 재확인 강제
verify --recheck  (같은 키 2회 실행 → 레코드 1개, 두 번째는 캐시된 응답. "두 번 결제됨"을 배제)
feedback(recheck) "확인된 실행에서는 레코드가 1개였고 두 번째 응답은 첫 응답과 같았습니다. 예측과 갈립니다."
ask:consequence "네 모델에서 이 실행 결과와 갈리는 가정은 어느 것일까요?"          (에피소드 시작)
answer          "서버가 두 요청을 구분 못 한다는 가정" / unsure
judge           partial / mechanism / vague   partial_credit="구분 여부가 핵심이라는 점"
ask:narrow      "서버가 두 요청이 같다는 걸 알 수 있는 정보가 요청 안에 있나요?"
answer          "헤더의 키" / unsure
judge           aligned / mechanism
ask:recombine   "그 키로 서버가 하는 일을 처음 질문에 다시 넣어 설명하면?"
answer          "같은 키면 이전 결과를 돌려주니 레코드 1개, 응답 동일" / sure
ask:retry(predict, 새 사례) "첫 요청이 서버에 도달조차 못 했다면 두 번째는?"
answer          "키가 처음이니 정상 처리, 레코드 1개" / sure
judge           aligned / mechanism / valid   → predict 통과 (도움 없는 시도)
episode close   cause=user_misconception (체크리스트 전부 충족)
ask:why         "왜 키 하나로 부수 효과가 한 번만 일어나죠? 순서대로 말해 보세요"
… boundary … → 개념 B로 이동 → B의 boundary 후 A의 transfer:
ask:transfer    "이메일 발송에 옮기면 결제의 어떤 요소가 어디에 대응하고, 대응 안 되는 건 뭘까요?"
answer          "키↔메시지 ID, 부수 효과↔발송, 결과 재사용↔이미 보냈다는 응답. 대응 안 되는 건 금액 같은 본문 변경" / sure
judge           aligned / mechanism / valid
feedback(개념)  "확인된 것: 키 매칭으로 부수 효과 1회. 범위: 같은 키 + 같은 본문. 미결: 키 만료 정책."
finalize        demonstrated / guided / consistent / retention untested / recheck_recommended
```

## 18. 인수 기준

구현이 지켜야 할 관찰 가능한 조건입니다.

- `own_words` 통과만으로는 `demonstrated`가 될 수 없다.
- 모든 hint·narrow 뒤에는 도움 없는 재시도가 존재한다.
- AI의 오류가 `user_misconception`으로 기록되지 않는다.
- 무효한 질문에 대한 답은 판정에 쓰이지 않는다.
- 모든 단계에서 `partial` 판정에 맞은 부분이 명시된다.
- `contested` 상태는 강제 결론 없이 `open`으로 끝날 수 있다.
- 설명 요청이 실패나 중단으로 보고되지 않는다.
- transfer 질문이 구조 대응을 요구한다.
- `prerequisite_gap`이 오개념으로 계산되지 않는다.
- 개념 종료 시 확인된 것과 미결이 명시된다.
- `leading` 질문이 연속되지 않는다.
- 부담 문항 수가 관리된다.

관찰해야 할 지표: 독립 transfer까지 걸린 질문 수 / 원인 오귀속 사례 수 / AI 질문 결함 수 / `leading` 문항 수 / 설명 후 반복과 적용이 구분되는지 / 이의 제기 후 대화가 방어적으로 흐르는지 / 보고가 "처음부터 앎"과 "지원 후 도달"을 구분하는지 / 종료 시 사용자가 확실한 것과 불확실한 것을 말할 수 있는지.

주의할 사용자 유형 세 가지: ① 지식은 있으나 표현이 서툰 사용자 ② 지식이 없고 솔직히 모른다고 말하는 사용자 ③ 강한 오개념과 높은 확신을 가진 사용자. 현재 설계는 ③에 강하고, ①·②에서 과잉 심문과 과잉 scaffold의 위험이 있습니다.

## 19. 알려진 한계

- **공유된 맹점** (§3.2) — 사용자와 AI가 같은 방식으로 틀리면 잡지 못합니다.
- **유지력 미검증** — 한 세션으로는 알 수 없습니다. 지연 확인 세션이 별도로 필요합니다.
- **단일 개념 세션의 transfer** — 지연 간격이 없어 검증력이 약합니다. 보고에 한계로 명시합니다.
- **개념 모드의 근거 의존** — 외부 근거의 품질이 세션의 품질을 정합니다.

## 다음 문서

- [상태 전이표](transitions.md) — 상태·사건별 전이의 전체 표. 구현과 테스트의 기준
- [핵심 규약](core.md) — 인식론적 불변조건, 응답 승인, 프로세스 간 통신, 저장 구조
