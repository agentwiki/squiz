#!/usr/bin/env sh
# squiz 바이너리 설치: GitHub 릴리스에서 플랫폼에 맞는 아카이브를 받아 설치한다.
# 이미 설치돼 있으면 그대로 사용한다. 성공 시 stdout에 바이너리 절대 경로 한 줄을 출력한다.
#
#   SQUIZ_INSTALL_DIR  설치 위치 (기본: $HOME/.local/bin)
#   SQUIZ_VERSION      특정 태그 고정 (기본: 최신 릴리스, 예: v0.1.0)
set -eu

REPO="agentwiki/squiz"
INSTALL_DIR="${SQUIZ_INSTALL_DIR:-$HOME/.local/bin}"

log() { printf '%s\n' "$*" >&2; }

exe=""
case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT) os=windows; exe=".exe" ;;
  *) log "지원하지 않는 OS: $(uname -s)"; exit 1 ;;
esac

if command -v squiz >/dev/null 2>&1; then
  command -v squiz
  exit 0
fi
if [ -x "$INSTALL_DIR/squiz$exe" ]; then
  printf '%s\n' "$INSTALL_DIR/squiz$exe"
  exit 0
fi

case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) log "지원하지 않는 아키텍처: $(uname -m)"; exit 1 ;;
esac

tag="${SQUIZ_VERSION:-}"
if [ -z "$tag" ]; then
  # releases/latest는 tag 페이지로 리다이렉트된다 — API 없이 최종 URL에서 태그를 읽는다
  tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" |
    sed -n 's#.*/releases/tag/##p')
fi
if [ -z "$tag" ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
fi
if [ -z "$tag" ]; then
  log "최신 릴리스를 찾지 못했습니다: https://github.com/$REPO/releases"
  exit 1
fi
version="${tag#v}"

ext="tar.gz"
[ "$os" = windows ] && ext="zip"
asset="squiz_${version}_${os}_${arch}.${ext}"
url="https://github.com/$REPO/releases/download/$tag/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

log "다운로드: $url"
curl -fsSL -o "$tmp/$asset" "$url"

# 체크섬 검증 (도구가 있을 때만)
if curl -fsSL -o "$tmp/checksums.txt" "https://github.com/$REPO/releases/download/$tag/checksums.txt" 2>/dev/null; then
  expected=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
  actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
  fi
  if [ -n "$actual" ] && [ -n "$expected" ] && [ "$actual" != "$expected" ]; then
    log "체크섬 불일치: $asset (expected $expected, got $actual)"
    exit 1
  fi
fi

# 추출: bsdtar(tar)는 zip도 처리한다
if [ "$ext" = zip ] && ! tar -tf "$tmp/$asset" >/dev/null 2>&1; then
  unzip -q "$tmp/$asset" -d "$tmp"
else
  tar -xzf "$tmp/$asset" -C "$tmp" 2>/dev/null || tar -xf "$tmp/$asset" -C "$tmp"
fi

if [ ! -f "$tmp/squiz$exe" ]; then
  log "아카이브에서 squiz$exe 를 찾지 못했습니다"
  exit 1
fi

mkdir -p "$INSTALL_DIR"
mv "$tmp/squiz$exe" "$INSTALL_DIR/squiz$exe"
chmod +x "$INSTALL_DIR/squiz$exe"
log "설치 완료: $INSTALL_DIR/squiz$exe ($tag)"
printf '%s\n' "$INSTALL_DIR/squiz$exe"
