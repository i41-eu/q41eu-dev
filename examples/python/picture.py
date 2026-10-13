#!/usr/bin/env python3
"""picture: a picture of yours on the robot's screen (or four numbered quarters), then the quarter
the person taps. Dev fits the picture to the screen; up to about 1.5 MB.

    python3 picture.py [photo.jpg]
"""
import base64, json, os, sys, urllib.request

URL, ROBOT, KEY = os.environ["SDEV_URL"].rstrip("/"), os.environ["SDEV_ROBOT"], os.environ["SDEV_KEY"]


def act(action, **args):
    req = urllib.request.Request(f"{URL}/v1/robots/{ROBOT}/{action}", data=json.dumps(args).encode(),
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as res:
        return json.load(res)


path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "img", "quarters.png")
with open(path, "rb") as f:
    image = base64.b64encode(f.read()).decode()
quarters = [{"id": str(i + 1), "rect": [160 * (i % 2), 120 * (i // 2), 160, 120]} for i in range(4)]
out = act("clickmap", image=image, regions=quarters)
print(f"quarter {out['region']} (at {out['x']}, {out['y']})" if out["region"] else "no tap within 30 s")
