#!/bin/sh
# menu: a snack menu of four tiles on the robot's screen; prints the one tapped (up to 30 s) and
# the robot says it.
#   sh menu.sh
set -eu
act() { # act <action> <JSON>: one action on the robot; prints its answer
  curl -sS --fail-with-body -X POST "$SDEV_URL/v1/robots/$SDEV_ROBOT/$1" \
    -H "Authorization: Bearer $SDEV_KEY" -H "Content-Type: application/json" -d "$2"
}
out=$(act menu '{"title":"Snack?","items":["Apple","Cake","Carrot","Milk"]}')
echo "$out"
item=$(echo "$out" | sed -n 's/.*"item":"\([^"]*\)".*/\1/p')
if [ -z "$item" ]; then echo "no choice within 30 s"; exit 1; fi
act show '{"clear":true}' >/dev/null # the speech bubble is on the face
act say "{\"text\":\"$item, yum!\"}" >/dev/null
