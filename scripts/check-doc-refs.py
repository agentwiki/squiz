#!/usr/bin/env python3
"""문서 참조가 실제로 존재하는지 검사한다.

두 가지를 본다:

1. Go 주석·메시지가 가리키는 문서 섹션(`design §14`, `transitions §2`,
   `core.md §3`)이 그 문서에 실재하는가.
2. 마크다운 문서끼리의 상대 링크가 실제 파일을 가리키는가.

섹션 번호는 문서를 고칠 때 밀리기 쉽고, 밀린 참조는 엉뚱한 내용을 가리키면서도
조용히 살아남는다. 그래서 사람이 아니라 CI가 본다.
"""

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SPECS = {
    "design": ROOT / "docs/spec/design.md",
    "transitions": ROOT / "docs/spec/transitions.md",
    "core": ROOT / "docs/spec/core.md",
}

HEADING = re.compile(r"^#{2,3}\s+(\d+(?:\.\d+)?)[.\s]")
DOCNAME = re.compile(r"\b(design|transitions|core)(?:\.md)?\b")
SECTION = re.compile(r"§(\d+(?:\.\d+)?)")

failures = []


def sections(path):
    """문서에서 `## 3.` / `### 3.1` 형태의 절 번호를 모은다."""
    found = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        m = HEADING.match(line)
        if m:
            found.add(m.group(1))
    return found


def check_go_refs(known):
    for path in sorted(ROOT.rglob("*.go")):
        for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            if "§" not in line:
                continue
            # 한 줄 안에서 각 §N을 그 앞에 가장 가까운 문서 이름에 붙인다.
            names = [(m.start(), m.group(1)) for m in DOCNAME.finditer(line)]
            for m in SECTION.finditer(line):
                doc = None
                for pos, name in names:
                    if pos < m.start():
                        doc = name
                if doc is None:
                    continue  # 문서를 특정할 수 없는 참조는 넘어간다
                if m.group(1) not in known[doc]:
                    rel = path.relative_to(ROOT)
                    failures.append(
                        f"{rel}:{n}: {doc}.md 에 §{m.group(1)} 이 없습니다 — {line.strip()}"
                    )


def check_md_links():
    for path in sorted(ROOT.rglob("*.md")):
        if "node_modules" in path.parts:
            continue
        for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            for m in re.finditer(r"\]\(([^)]+)\)", line):
                target = m.group(1)
                if target.startswith(("http://", "https://", "#", "mailto:")):
                    continue
                resolved = (path.parent / target.split("#")[0]).resolve()
                if not resolved.exists():
                    rel = path.relative_to(ROOT)
                    failures.append(f"{rel}:{n}: 링크 대상이 없습니다 — {target}")


def main():
    known = {}
    for name, path in SPECS.items():
        if not path.exists():
            print(f"명세 문서가 없습니다: {path}", file=sys.stderr)
            return 1
        known[name] = sections(path)

    check_go_refs(known)
    check_md_links()

    if failures:
        for f in failures:
            print(f"::error::{f}" if "--ci" in sys.argv else f, file=sys.stderr)
        print(f"\n문서 참조 {len(failures)}건이 깨졌습니다.", file=sys.stderr)
        return 1

    total = sum(len(v) for v in known.values())
    print(f"문서 참조 검사 통과 (명세 절 {total}개, 깨진 참조 없음)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
