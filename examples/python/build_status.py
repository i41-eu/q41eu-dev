#!/usr/bin/env python3
"""build_status: runs a command and shows on the robot how it went: blue while it runs, then a
green card and a chime when it passed, a red card and a shake when it failed.

    python3 build_status.py make test
"""
import json, os, subprocess, sys, time, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]


def act(action, **args):
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(),
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


cmd = sys.argv[1:] or ["make", "test"]
name = " ".join(cmd)
act("show", title="Running", text=name, color="blue")
act("leds", effect="breathe", color="blue")
start = time.time()
code = subprocess.call(cmd)
took = f"{name} · {time.time() - start:.0f} s"
if code == 0:
    act("show", title="Passed", text="All good", note=took, color="green", progress=1)
    act("leds", color="green")
    act("sound", chime="tada")
    act("move", gesture="nod")
else:
    act("show", title="Failed", text="Have a look", note=took, color="red")
    act("leds", color="red")
    act("sound", chime="error")
    act("move", gesture="shake")
sys.exit(code)
