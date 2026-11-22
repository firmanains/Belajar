#!/usr/bin/env bash
# Run the SAA "next" workflow several times in a row, unattended.
# Each run is a fresh Claude Code session (same effect as /clear + next).
#
# Usage (from your SAA notes folder):
#   bash run-saa-loop.sh            # 3 topics
#   bash run-saa-loop.sh 5          # 5 topics
#   bash run-saa-loop.sh 5 "budget 12"   # extra options passed to every run
#
# Stops early when: all topics are done, a run fails (e.g. usage limit),
# Claude reports "Budget too small", or a run did not tick any to-do item.

set -uo pipefail

RUNS="${1:-3}"
OPTIONS="${2:-}"
PAUSE_SECONDS=15
LOG_DIR=".saa-logs"   # dot-folder: hidden in Obsidian, never staged by the Git workflow

if [ ! -f CLAUDE.md ] || [ ! -f _todo.md ]; then
  echo "Run this inside your SAA notes folder (CLAUDE.md and _todo.md must exist)."
  exit 1
fi
command -v claude >/dev/null || { echo "claude CLI not found in PATH."; exit 1; }

PROMPT="next"
[ -n "$OPTIONS" ] && PROMPT="next, $OPTIONS"

# Tools Claude may use without asking. Nothing else is auto-approved.
# If your Claude Code version uses the older colon syntax, change
# "Bash(git add *)" to "Bash(git add:*)" etc. (check: claude --help)
ALLOWED_TOOLS=(
  "Read" "Write" "Edit" "Glob" "Grep" "WebSearch"
  "WebFetch(domain:docs.aws.amazon.com)"
  "WebFetch(domain:aws.amazon.com)"
  "WebFetch(domain:d1.awsstatic.com)"
  "WebFetch(domain:tutorialsdojo.com)"
  "Bash(git status *)" "Bash(git rev-parse *)"
  "Bash(git add *)" "Bash(git commit *)"
  "Bash(git push)" "Bash(git push *)"
  "Bash(python3 *)" "Bash(mkdir *)" "Bash(ls *)"
)

count_open() { grep -c '^- \[ \]' _todo.md || true; }

mkdir -p "$LOG_DIR"

for ((i = 1; i <= RUNS; i++)); do
  before=$(count_open)
  if [ "$before" -eq 0 ]; then
    echo "All to-do items are done. 🎉"
    break
  fi

  log="$LOG_DIR/run-$(date +%Y%m%d-%H%M%S).md"
  echo "=== Run $i/$RUNS · $before topics left · prompt: \"$PROMPT\" · log: $log"

  claude -p "$PROMPT" \
    --model sonnet \
    --max-turns 80 \
    --allowedTools "${ALLOWED_TOOLS[@]}" \
    > "$log" 2>&1
  rc=$?

  tail -n 25 "$log"
  echo

  if [ $rc -ne 0 ]; then
    echo "STOP: claude exited with code $rc (usage limit, network, or error). See $log"
    break
  fi
  if grep -q "Budget too small" "$log"; then
    echo "STOP: Claude asked for a bigger budget. Re-run that topic manually with the suggested budget."
    break
  fi

  after=$(count_open)
  if [ "$after" -ge "$before" ]; then
    echo "STOP: no to-do item was ticked in this run. Read $log before continuing."
    break
  fi

  if [ $i -lt "$RUNS" ]; then
    sleep "$PAUSE_SECONDS"
  fi
done

echo "Finished. $(count_open) topics left. Logs are in $LOG_DIR/"