"""Measures how long an editor takes to show a file, from the start of its
process to the first frame that looks like the settled window, and how much
memory its processes use. It sends no input: it starts the editor with the file
as an argument, keeps its window above the others, and grabs the window.

usage: measure.py name exe seconds out_dir -- args...
"""
import csv
import ctypes
import ctypes.wintypes as wt
import io
import json
import os
import subprocess
import sys
import threading
import time

import numpy as np
from PIL import ImageGrab

user32 = ctypes.windll.user32
kernel32 = ctypes.windll.kernel32
dwmapi = ctypes.windll.dwmapi
ctypes.windll.shcore.SetProcessDpiAwareness(2)

SNAPS = [float(x) for x in os.environ.get("MEASURE_SNAP", "").split(",") if x]
LIMIT_MB = 6000  # give up when the editor's processes hold more


def image_of(pid):
    h = kernel32.OpenProcess(0x1000, False, pid)
    if not h:
        return ""
    buf = ctypes.create_unicode_buffer(1024)
    n = wt.DWORD(1024)
    ok = kernel32.QueryFullProcessImageNameW(h, 0, buf, ctypes.byref(n))
    kernel32.CloseHandle(h)
    return os.path.basename(buf.value).lower() if ok else ""


def windows_of(image):
    found = []

    @ctypes.WINFUNCTYPE(wt.BOOL, wt.HWND, wt.LPARAM)
    def each(hwnd, _):
        if not user32.IsWindowVisible(hwnd):
            return True
        pid = wt.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        if image_of(pid.value) != image:
            return True
        r = rect_of(hwnd)
        if r[2] - r[0] > 400 and r[3] - r[1] > 300:
            found.append((hwnd, r))
        return True

    user32.EnumWindows(each, 0)
    return found


def rect_of(hwnd):
    r = wt.RECT()
    if dwmapi.DwmGetWindowAttribute(hwnd, 9, ctypes.byref(r), ctypes.sizeof(r)) != 0:
        user32.GetWindowRect(hwnd, ctypes.byref(r))
    return (r.left, r.top, r.right, r.bottom)


def memory_mb(image):
    out = subprocess.run(["tasklist", "/FO", "CSV", "/NH", "/FI", "IMAGENAME eq " + image],
                         capture_output=True, text=True, encoding="cp932", errors="replace").stdout
    total = 0
    for row in csv.reader(io.StringIO(out)):
        if len(row) >= 5 and row[0].lower() == image:
            digits = "".join(c for c in row[4] if c.isdigit())
            if digits:
                total += int(digits)
    return total / 1024


def main():
    sep = sys.argv.index("--")
    name, exe, seconds, out_dir = sys.argv[1:sep]
    args = sys.argv[sep + 1:]
    seconds = float(seconds)
    image = os.path.basename(exe).lower()
    os.makedirs(out_dir, exist_ok=True)
    if memory_mb(image) > 0:
        print(json.dumps({"name": name, "error": image + " is already running; not measured"}))
        return

    mem = {"peak": 0.0, "last": 0.0, "stop": False, "over": False}

    def watch():
        while not mem["stop"]:
            m = memory_mb(image)
            mem["last"] = m
            mem["peak"] = max(mem["peak"], m)
            if m > LIMIT_MB:
                mem["over"] = True
            time.sleep(0.5)

    env = dict(os.environ)
    if os.environ.get("MEASURE_APPDATA"):
        env["APPDATA"] = os.environ["MEASURE_APPDATA"]
    ImageGrab.grab(bbox=(0, 0, 64, 64))  # the first grab is slow
    t0 = time.perf_counter()
    subprocess.Popen([exe] + args, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    threading.Thread(target=watch, daemon=True).start()

    hwnd, window_at = None, None
    frames = []  # (t, rect, small gray array)
    last_full = None
    while time.perf_counter() - t0 < seconds and not mem["over"]:
        if hwnd is None or not user32.IsWindow(hwnd):
            ws = windows_of(image)
            if not ws:
                time.sleep(0.005)
                continue
            hwnd = max(ws, key=lambda w: (w[1][2] - w[1][0]) * (w[1][3] - w[1][1]))[0]
            if window_at is None:
                window_at = time.perf_counter() - t0
            # above the others, without moving, sizing or activating it
            user32.SetWindowPos(hwnd, wt.HWND(-1), 0, 0, 0, 0, 0x0001 | 0x0002 | 0x0010)
        r = rect_of(hwnd)
        if r[2] - r[0] < 400:
            continue
        img = ImageGrab.grab(bbox=r)
        t = time.perf_counter() - t0
        last_full = img
        while SNAPS and t >= SNAPS[0]:
            img.save(os.path.join(out_dir, "%s-at%04.1f.png" % (name, SNAPS.pop(0))))
        if os.environ.get("MEASURE_DEBUG") and len(frames) in (0, 3, 10):
            img.save(os.path.join(out_dir, "%s-f%d.png" % (name, len(frames))))
        small = np.asarray(img.convert("L").resize((320, 200)), dtype=np.int16)
        frames.append((t, r, small, img if len(frames) % 1 == 0 else None))
        # keep only a few full images in memory
        if len(frames) > 3:
            t_, r_, s_, _ = frames[-3]
            frames[-3] = (t_, r_, s_, None)
    mem["stop"] = True
    time.sleep(0.6)
    result = {"name": name, "window_ms": None if window_at is None else round(window_at * 1000),
              "peak_mb": round(mem["peak"]), "end_mb": round(mem["last"]), "over_limit": mem["over"],
              "frames": len(frames)}
    if frames:
        final_rect, final = frames[-1][1], frames[-1][2]
        last_bad = -1
        fracs = []
        for i, (t, r, s, _) in enumerate(frames):
            if r != final_rect:
                last_bad = i
                fracs.append(1.0)
                continue
            frac = float((np.abs(s - final) > 24).mean())
            fracs.append(frac)
            if frac >= 0.03:
                last_bad = i
        settled = frames[min(last_bad + 1, len(frames) - 1)][0]
        # The text is "shown" from the first frame whose rows hold as much ink
        # as those of the final frame: a profile by rows does not change when
        # the text moves sideways, as it does when the gutter grows wider.
        def rows(a):
            return (np.abs(a - np.median(a)) > 40).sum(axis=1).astype(np.float64)
        pf = rows(final)
        last_bad = -1
        for i, (t, r, sm, _) in enumerate(frames):
            d = np.abs(rows(sm) - pf).sum() / max(pf.sum(), 1.0) if r == final_rect else 1.0
            fracs[i] = float(d)
            if d >= 0.15:
                last_bad = i
        shown = frames[min(last_bad + 1, len(frames) - 1)][0]
        result["settled_ms"] = round(settled * 1000)
        result["ink_rows"] = int((pf > 3).sum())
        if os.environ.get("MEASURE_DEBUG"):
            result["timeline"] = [(round(frames[i][0] * 1000), round(fracs[i], 3)) for i in range(min(45, len(frames)))]
        result["shown_ms"] = round(shown * 1000)
        result["first_frame_ms"] = round(frames[0][0] * 1000)
        result["grab_ms"] = round((frames[-1][0] - frames[0][0]) / max(1, len(frames) - 1) * 1000)
        result["settled_for_s"] = round(frames[-1][0] - shown, 1)
        last_full.save(os.path.join(out_dir, name + "-final.png"))
    subprocess.run(["taskkill", "/IM", image, "/F", "/T"], capture_output=True)
    time.sleep(1.5)
    print(json.dumps(result))


main()
