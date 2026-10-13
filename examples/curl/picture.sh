#!/bin/sh
# picture: a picture of yours on the robot's screen (or four numbered quarters), then the quarter
# the person taps (up to 30 s). Dev fits the picture to the screen; up to about 1.5 MB.
#   sh picture.sh [photo.jpg]
set -eu
pic=${1:-$(dirname "$0")/../img/quarters.png}
quarters='[{"id":"1","rect":[0,0,160,120]},{"id":"2","rect":[160,0,160,120]},{"id":"3","rect":[0,120,160,120]},{"id":"4","rect":[160,120,160,120]}]'
printf '{"image":"%s","regions":%s}' "$(base64 < "$pic" | tr -d '\n')" "$quarters" |
  curl -sS --fail-with-body -X POST "$SDEV_URL/v1/robots/$SDEV_ROBOT/clickmap" \
    -H "Authorization: Bearer $SDEV_KEY" -H "Content-Type: application/json" --data-binary @-
echo
