#!/usr/bin/env python3
"""events: prints what people do at the robot and answers some of it: a pat on the head makes it
chirp and smile, a tag held near shows what the tag says. Ctrl+C ends it.

    python3 events.py
"""
import json, os, time, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]
HEADERS = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}


def act(action, **args):
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(), headers=HEADERS)
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


def events(after, seconds):
    """The events after a sequence number; waits up to seconds for the next one."""
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/events?after={after}&seconds={seconds}", headers=HEADERS)
    with urllib.request.urlopen(req, timeout=seconds + 30) as res:
        return json.load(res)


last = events(0, 0)["last_seq"]  # start from now
print("touch the robot: tap its screen, pat its head, hold a tag near it…")
while True:
    try:
        got = events(last, 30)
    except OSError as err:
        print(err)
        time.sleep(3)
        continue
    last = got["last_seq"]
    for e in got["events"]:
        print(e["name"], e.get("data", {}))
        if e["name"] in ("pat", "pet", "stroke"):
            act("face", emotion="happy")
            act("sound", chime="chirp")
        elif e["name"] == "tag":
            act("show", title="A tag", text=e["data"].get("text") or "(no text)", note=e["data"].get("uid", ""))
        elif e["name"] == "tap":
            act("show", clear=True)
