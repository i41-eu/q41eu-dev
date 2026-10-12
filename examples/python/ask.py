#!/usr/bin/env python3
"""ask: a question on the robot's screen; the person at the robot taps an answer.

    python3 ask.py "Coffee or tea?" Coffee Tea
"""
import json, os, sys, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]


def act(action, **args):
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(),
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


question, answers = (sys.argv[1], sys.argv[2:]) if len(sys.argv) > 3 else ("Coffee or tea?", ["Coffee", "Tea"])
out = act("ask", question=question, answers=answers)
print(out["answer"] or "no answer within 30 s")
