#!/usr/bin/env bash
set -euo pipefail

content=$(cat | tr -d '\r')
# Update tmux buffer and system clipboard (uses tmux set-clipboard setting)
tmux set-buffer -w -- "$content"

# Best-effort: also call platform clip utilities when available
if command -v pbcopy >/dev/null 2>&1; then
  printf '%s' "$content" | pbcopy || true
elif command -v wl-copy >/dev/null 2>&1; then
  printf '%s' "$content" | wl-copy --type text || wl-copy || true
elif command -v xclip >/dev/null 2>&1; then
  printf '%s' "$content" | xclip -selection clipboard || true
elif command -v xsel >/dev/null 2>&1; then
  printf '%s' "$content" | xsel --clipboard --input || true
elif command -v powershell.exe >/dev/null 2>&1; then
  powershell.exe -NoProfile -Command Set-Clipboard -Value @"
${content}
"@ || true
fi

