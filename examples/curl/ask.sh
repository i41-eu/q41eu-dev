#!/bin/sh
# ask: a question on the robot's screen; the answer the person taps comes back (up to 30 s).
#   sh ask.sh
set -eu
curl -sS --fail-with-body -X POST "$SDEV_URL/v1/robots/$SDEV_ROBOT/ask" \
  -H "Authorization: Bearer $SDEV_KEY" -H "Content-Type: application/json" \
  -d '{"question":"Coffee or tea?","answers":["Coffee","Tea"]}'
echo
