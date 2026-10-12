#!/bin/sh
# hello: a card on the robot's screen, its LEDs, a nod and a chime.
#   export SDEV_URL=… SDEV_ROBOT=… SDEV_KEY=…   (Dev's page shows them with a new key)
#   sh hello.sh
set -eu
act() { # act <action> <JSON>: one action on the robot
  curl -sS --fail-with-body -X POST "$SDEV_URL/v1/robots/$SDEV_ROBOT/$1" \
    -H "Authorization: Bearer $SDEV_KEY" -H "Content-Type: application/json" -d "$2"
  echo
}
act show '{"title":"Hello","text":"from curl","note":"q41eu-dev examples","color":"green"}'
act leds '{"color":"green"}'
act move '{"gesture":"nod"}'
act sound '{"chime":"tada"}'
