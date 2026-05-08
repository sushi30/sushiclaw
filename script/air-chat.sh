#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHELL_BIN="${SHELL:-/bin/sh}"
HOST="${AIR_CHAT_GATEWAY_HOST:-127.0.0.1}"
PORT="${AIR_CHAT_GATEWAY_PORT:-18801}"
SESSION_ID="${AIR_CHAT_SESSION_ID:-cli}"
TMP_DIR="${TMPDIR:-/tmp}/sushiclaw-air-chat"
TMP_CONFIG="${TMP_DIR}/config.json"

if [ -n "${SUSHICLAW_CONFIG:-}" ]; then
  BASE_CONFIG="${SUSHICLAW_CONFIG}"
elif [ -n "${SUSHICLAW_HOME:-}" ]; then
  BASE_CONFIG="${SUSHICLAW_HOME}/config.json"
elif [ -n "${PICOCLAW_HOME:-}" ]; then
  BASE_CONFIG="${PICOCLAW_HOME}/config.json"
else
  BASE_CONFIG="${HOME}/.picoclaw/config.json"
fi

if [ ! -f "${BASE_CONFIG}" ]; then
  echo "Config not found: ${BASE_CONFIG}" >&2
  exit 1
fi

mkdir -p "${TMP_DIR}"
TOKEN="$(openssl rand -hex 16 2>/dev/null || printf '%s-%s' "$$" "$(date +%s)")"
jq \
  --arg token "${TOKEN}" \
  --argjson port "${PORT}" \
  '.channels = (.channels // {}) |
   .channels.websocket = ((.channels.websocket // {}) + {
     "enabled": true,
     "type": "websocket",
     "token": $token,
     "allow_origins": ["*"],
     "allow_from": ["*"],
     "port": $port
   })' \
  "${BASE_CONFIG}" > "${TMP_CONFIG}"

AIR_CMD="SUSHICLAW_CONFIG='${TMP_CONFIG}' air; exec ${SHELL_BIN} -i"
CHAT_CMD="SUSHICLAW_CONFIG='${TMP_CONFIG}' go run -tags whatsapp_native . chat --debug --gateway-host '${HOST}' --gateway-port '${PORT}' --gateway-token '${TOKEN}' --gateway-session '${SESSION_ID}'; exec ${SHELL_BIN} -i"

if [ -n "${TMUX:-}" ]; then
  SESSION="$(tmux display-message -p '#S')"
  WINDOW="$(tmux display-message -p '#I')"
  CURRENT_PANE="$(tmux display-message -p '#P')"

  tmux split-window -h -t "${SESSION}:${WINDOW}.${CURRENT_PANE}" -c "${ROOT}" "${AIR_CMD}"
  tmux select-pane -t "${SESSION}:${WINDOW}.${CURRENT_PANE}"

  echo "Started Air gateway in a new tmux pane."
  cd "${ROOT}"
  exec bash -lc "${CHAT_CMD}"
fi

SESSION="sushiclaw-air-chat"

if tmux has-session -t "${SESSION}" 2>/dev/null; then
  echo "tmux session '${SESSION}' already exists; attaching."
  exec tmux attach-session -t "${SESSION}"
fi

tmux new-session -d -s "${SESSION}" -c "${ROOT}" "${CHAT_CMD}"
tmux split-window -h -t "${SESSION}:0.0" -c "${ROOT}" "${AIR_CMD}"
tmux select-pane -t "${SESSION}:0.0"
exec tmux attach-session -t "${SESSION}"
