#!/bin/sh
# events: waits up to 30 s for the next thing a person does at the robot (tap, pat, a tag, …).
# The answer's last_seq goes into after= next time:
#   sh events.sh          (from the last ones kept)
#   AFTER=12 sh events.sh
set -eu
curl -sS --fail-with-body "$SDEV_URL/v1/robots/$SDEV_ROBOT/events?after=${AFTER:-0}&seconds=30" \
  -H "Authorization: Bearer $SDEV_KEY"
echo
