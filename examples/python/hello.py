#!/usr/bin/env python3
"""hello: a card on the robot's screen, its LEDs, a nod and a chime.

    export SDEV_URL=… SDEV_ROBOT=… SDEV_KEY=…   (Dev's page shows them with a new key)
    python3 hello.py
Only the standard library.
"""
import json, os, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]


def act(action, **args):
    """One action on the robot: POST /v1/robots/<robot>/<action> with the arguments as JSON."""
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(),
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


act("show", title="Hello", text="from Python", note="q41eu-dev examples", color="green")
act("leds", color="green")
act("move", gesture="nod")
act("sound", chime="tada")
print("done: look at the robot")
