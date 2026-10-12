#!/bin/sh
# build_status: runs a command and shows on the robot how it went: blue while it runs, then a
# green card and a chime when it passed, a red card and a shake when it failed. Its exit code is
# the command's.
#   sh build_status.sh make test
set -u
act() { # act <action> <JSON>: one action on the robot
  curl -sS --fail-with-body -X POST "$SDEV_URL/v1/robots/$SDEV_ROBOT/$1" \
    -H "Authorization: Bearer $SDEV_KEY" -H "Content-Type: application/json" -d "$2"
  echo
}
[ $# -gt 0 ] || set -- make test
name=$(printf '%s' "$*" | sed 's/\\/\\\\/g; s/"/\\"/g') # the command, as a JSON string
act show "{\"title\":\"Running\",\"text\":\"$name\",\"color\":\"blue\"}"
act leds '{"effect":"breathe","color":"blue"}'
start=$(date +%s)
if "$@"; then code=0; else code=$?; fi
took="$name · $(($(date +%s) - start)) s"
if [ "$code" -eq 0 ]; then
  act show "{\"title\":\"Passed\",\"text\":\"All good\",\"note\":\"$took\",\"color\":\"green\",\"progress\":1}"
  act leds '{"color":"green"}'
  act sound '{"chime":"tada"}'
  act move '{"gesture":"nod"}'
else
  act show "{\"title\":\"Failed\",\"text\":\"Have a look\",\"note\":\"$took\",\"color\":\"red\"}"
  act leds '{"color":"red"}'
  act sound '{"chime":"error"}'
  act move '{"gesture":"shake"}'
fi
exit "$code"
