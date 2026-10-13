#!/usr/bin/env python3
"""menu: a snack menu of four tiles on the robot's screen; prints the one tapped and the robot
says it.

    python3 menu.py
"""
import json, os, sys, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]


def act(action, **args):
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(),
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


out = act("menu", title="Snack?", items=["Apple", "Cake", "Carrot", "Milk"])
if not out["item"]:
    sys.exit("no choice within 30 s")
print(out["item"])
act("show", clear=True)  # the speech bubble is on the face
act("say", text=f"{out['item']}, yum!")
