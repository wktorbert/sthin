#!/usr/bin/env bash
# oneshot.sh — one command, two phases.
#
#   Phase 1  /oneshot-plan   idea  -> CONTEXT.md, SPEC.md, tickets, verify.sh, GOAL.txt
#   Phase 2  /goal           state -> a built, verified application
#
# Usage:
#   ./oneshot.sh "a CLI that watches a directory and mirrors it to S3"
#   ./oneshot.sh --plan-only "..."       # stop after Phase 1
#   ./oneshot.sh --build-only            # skip Phase 1, use existing .oneshot/
#   ./oneshot.sh --resume                # re-arm the goal after a crash
#
# Env:
#   ONESHOT_TURNS=40         turn budget written into the goal condition
#   ONESHOT_AUTONOMY=assisted   assisted | interactive | auto
#   ONESHOT_YOLO=1           pass --dangerously-skip-permissions to Phase 2
#   ONESHOT_CLAUDE_ARGS=""   extra args appended to every claude invocation

set -euo pipefail

TURNS="${ONESHOT_TURNS:-40}"
AUTONOMY="${ONESHOT_AUTONOMY:-assisted}"
STATE=".oneshot"
EXTRA="${ONESHOT_CLAUDE_ARGS:-}"

PLAN_ONLY=0
BUILD_ONLY=0
IDEA=""

while [ $# -gt 0 ]; do
  case "$1" in
    --plan-only)  PLAN_ONLY=1; shift ;;
    --build-only) BUILD_ONLY=1; shift ;;
    --resume)     BUILD_ONLY=1; shift ;;
    --turns)      TURNS="$2"; shift 2 ;;
    -h|--help)    sed -n '2,20p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *)            IDEA="$1"; shift ;;
  esac
done

command -v claude >/dev/null || { echo "claude CLI not found on PATH"; exit 1; }
command -v jq     >/dev/null || echo "note: jq not found — stream output will be raw JSON"

# ── Phase 1 ────────────────────────────────────────────────────────────────
if [ "$BUILD_ONLY" -eq 0 ]; then
  [ -n "$IDEA" ] || { echo "usage: ./oneshot.sh \"<what you want built>\""; exit 1; }

  echo "▸ Phase 1: planning ($AUTONOMY)"
  PLAN_PROMPT="/oneshot-plan [autonomy=$AUTONOMY turns=$TURNS] $IDEA"
  if [ "$AUTONOMY" = "interactive" ]; then
    # -p is headless and can never ask you anything. Interactive planning
    # needs a real session; exit it when the plan is written.
    claude "$PLAN_PROMPT" $EXTRA
  else
    claude -p "$PLAN_PROMPT" $EXTRA
  fi

  for f in "$STATE/GOAL.txt" "$STATE/SPEC.md" "verify.sh"; do
    [ -f "$f" ] || { echo "✗ Phase 1 did not produce $f — not arming the goal."; exit 1; }
  done
  chmod +x verify.sh

  # A harness that already passes against an unbuilt repo would end the loop
  # on turn one. Catch that here rather than three hours from now.
  if ./verify.sh 2>/dev/null | grep -q "ONESHOT""-VERIFY: PASS"; then
    echo "✗ verify.sh passes against the current tree. Either the work is done,"
    echo "  or the harness is checking nothing. Inspect it before continuing."
    exit 1
  fi

  echo
  echo "── riskiest assumptions ───────────────────────────────────────────"
  sed -n '1,60p' "$STATE/ASSUMPTIONS.md" 2>/dev/null || echo "(none written)"
  echo "───────────────────────────────────────────────────────────────────"
  echo
fi

[ "$PLAN_ONLY" -eq 1 ] && { echo "▸ Stopping after Phase 1 (--plan-only)."; exit 0; }
[ -f "$STATE/GOAL.txt" ] || { echo "No $STATE/GOAL.txt — run Phase 1 first."; exit 1; }

# ── Confirmation gate ──────────────────────────────────────────────────────
if [ "$AUTONOMY" != "auto" ] && [ -t 0 ]; then
  echo "Goal condition:"
  sed 's/^/  /' "$STATE/GOAL.txt"
  echo
  read -r -p "Arm the goal loop (up to $TURNS turns)? [y/N] " ok
  [[ "$ok" =~ ^[Yy]$ ]] || { echo "Aborted. State is in $STATE/ — resume with --resume."; exit 0; }
fi

# ── Phase 2 ────────────────────────────────────────────────────────────────
YOLO=""
if [ "${ONESHOT_YOLO:-0}" = "1" ]; then
  YOLO="--dangerously-skip-permissions"
elif ! grep -qs '"allow"' .claude/settings.json .claude/settings.local.json 2>/dev/null; then
  # In -p mode there is nobody to answer a permission prompt, so an
  # unallowlisted tool call is denied, not asked. A goal loop with no
  # allowlist and no YOLO flag will fail on its first Bash call.
  echo "⚠  No permissions.allow found in .claude/settings*.json and ONESHOT_YOLO is unset."
  echo "   Headless turns cannot prompt you; unallowlisted tools are denied outright."
  echo "   Either add an allowlist (Bash, Edit, Write, Read at minimum) or set ONESHOT_YOLO=1."
  if [ -t 0 ]; then
    read -r -p "   Continue anyway? [y/N] " ok
    [[ "$ok" =~ ^[Yy]$ ]] || exit 1
  fi
fi

echo "▸ Phase 2: goal loop"
# Flatten to one line. A multi-line argument to a slash command may fail to
# parse as /goal at all, in which case the whole thing is read as an ordinary
# prompt, no goal is armed, and the run stops after a single turn.
GOAL="$(tr '\n' ' ' < "$STATE/GOAL.txt" | tr -s ' ' | sed 's/[[:space:]]*$//')"
if [ "$(printf '%s' "$GOAL" | wc -l | tr -d ' ')" != "0" ]; then
  echo "✗ Goal condition still contains a newline."; exit 1
fi
[ ${#GOAL} -le 4000 ] || { echo "✗ Goal condition is ${#GOAL} chars; the limit is 4000."; exit 1; }

if command -v jq >/dev/null; then
  claude -p "/goal $GOAL" --output-format stream-json --verbose $YOLO $EXTRA \
    | jq -r --unbuffered 'if .type=="assistant" then
        (.message.content[]? | select(.type=="text") | .text)
      elif .type=="result" then "\n▸ " + (.subtype // "done")
      else empty end' \
    | tee -a "$STATE/build.log"
else
  claude -p "/goal $GOAL" --output-format stream-json --verbose $YOLO $EXTRA \
    | tee -a "$STATE/build.log"
fi

# ── Verdict ────────────────────────────────────────────────────────────────
echo
echo "▸ Final verification"
if ./verify.sh; then
  echo "✓ Done. Spec: $STATE/SPEC.md — progress: $STATE/PROGRESS.md"
else
  echo "✗ Not there yet. Read $STATE/PROGRESS.md, then ./oneshot.sh --resume"
  exit 1
fi