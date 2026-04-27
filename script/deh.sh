#!/usr/bin/env bash
set -euo pipefail

# Validate we are inside a tmux session
if [ -z "${TMUX:-}" ]; then
  echo "Error: not running inside a tmux session." >&2
  exit 1
fi

# Get current session and window
SESSION=$(tmux display-message -p '#S')
WINDOW=$(tmux display-message -p '#I')

# Split current pane vertically and run air in the new pane
tmux split-window -v -t "${SESSION}:${WINDOW}" "air"

# Give air a moment to start
sleep 2

# Capture last 5 lines from the new (bottom/right) pane to validate server is running
NEW_PANE=$(tmux display-message -p '#P')
# Since we split, the new pane is usually the one with the highest index or active pane.
# We'll capture from the last pane in the window.
PANES=$(tmux list-panes -t "${SESSION}:${WINDOW}" -F '#P')
TARGET_PANE=$(echo "$PANES" | tail -n 1)

echo "--- Air pane log (last 5 lines) ---"
tmux capture-pane -t "${SESSION}:${WINDOW}.${TARGET_PANE}" -p | tail -n 5

echo "---"
echo "Air is running in pane ${TARGET_PANE} of window ${WINDOW}."
