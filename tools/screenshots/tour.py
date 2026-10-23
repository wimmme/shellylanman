"""The README tour: a short recording of a running ShellyLanMan with simulated
devices (Devices → Checklist → Firmware → Charts).

    SCRIPT=tour.py sh tools/screenshots/run.sh && sh tools/screenshots/video.sh

Writes one PNG per screen change to /shots/tour/ and an ffmpeg concat list with
how long each one shows (concat.txt); video.sh turns them into tour.mp4 and
tour.gif. Waits for loading (the Firmware index, chart history) are cut out.
"""
import os
import time

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
DIR = "/shots/tour/"
W, H = 1280, 800
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"
# Columns that do not fit at this width, as in shoot.py.
COLS = """try { localStorage.setItem('sl_cols_default', JSON.stringify(['keyword', 'mac', 'ssid', 'logs',
  'device', 'mqtt', 'uptime', 'source', 'cloud', 'hostname'])) } catch (e) {}"""
# A recording shows no mouse pointer: draw one that follows the mouse.
POINTER = """addEventListener('DOMContentLoaded', () => {
  const p = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  p.setAttribute('viewBox', '0 0 24 24'); p.setAttribute('width', '26'); p.setAttribute('height', '26');
  const a = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  a.setAttribute('d', 'M3 2l7.5 19 2.6-7.9 7.9-2.6z');
  a.setAttribute('fill', '#fff'); a.setAttribute('stroke', '#000'); a.setAttribute('stroke-width', '1.4');
  p.appendChild(a);
  Object.assign(p.style, { position: 'fixed', left: '-40px', top: '-40px', zIndex: 2147483647,
    pointerEvents: 'none', transition: 'transform 120ms' });
  document.documentElement.appendChild(p);
  addEventListener('mousemove', e => { p.style.left = (e.clientX - 3) + 'px'; p.style.top = (e.clientY - 2) + 'px'; }, true);
  addEventListener('mousedown', () => { p.style.transform = 'scale(0.8)'; }, true);
  addEventListener('mouseup', () => { p.style.transform = ''; }, true);
});"""

os.makedirs(DIR, exist_ok=True)
os.chmod(DIR, 0o777)  # created as root: the next tools/remote.sh run must be able to remove it
frames = []  # [file, seconds]
last = None
mouse = [W / 2, H / 2]


def capture(pg, cut=False):
    """Takes a frame; the previous one lasts until now (or briefly, after a cut)."""
    global last
    now = time.monotonic()
    if frames:
        frames[-1][1] = 0.1 if cut else now - last
    name = f"{len(frames):04d}.png"
    pg.screenshot(path=DIR + name)
    frames.append([name, 0.1])
    last = time.monotonic() if cut else now


def hold(pg, seconds):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        capture(pg)


def move(pg, loc, steps=10):
    box = loc.bounding_box()
    x, y = box["x"] + box["width"] / 2, box["y"] + box["height"] / 2
    x0, y0 = mouse
    for i in range(1, steps + 1):
        f = i / steps
        f = f * f * (3 - 2 * f)
        pg.mouse.move(x0 + (x - x0) * f, y0 + (y - y0) * f)
        capture(pg)
    mouse[:] = [x, y]


def click(pg, loc):
    move(pg, loc)
    pg.mouse.down(); capture(pg)
    pg.mouse.up(); capture(pg)


def row(pg, name):
    return pg.locator("table.devices tbody tr", has_text=name)


def nav(pg, label):
    click(pg, pg.locator(".nav a", has_text=label))


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": W, "height": H})
    for s in (QUIET, COLS, POINTER):
        ctx.add_init_script(s)
    pg = ctx.new_page()

    # Warm up: the Firmware page fetches Shelly's index once, the charts need history.
    pg.goto(B + "#/firmware"); pg.wait_for_timeout(8000)
    pg.wait_for_timeout(30000)
    pg.goto(B + "#/devices"); pg.reload(); pg.wait_for_timeout(3000)
    pg.mouse.move(*mouse)

    hold(pg, 2.5)
    # Scroll the table up, then tick devices further down as they come into view.
    pg.evaluate("document.querySelector('.main').scrollTo({ top: 240, behavior: 'smooth' })")
    hold(pg, 1)
    for name in ("Garden pump", "Porch light", "Living room", "Heat pump"):
        box = row(pg, name).locator("td.sel input")
        if box.bounding_box()["y"] > H - 60:
            pg.evaluate("n => document.querySelector('.main').scrollBy({ top: n, behavior: 'smooth' })",
                        box.bounding_box()["y"] - H / 2)
            hold(pg, 0.8)
        click(pg, box)
        hold(pg, 0.3)
    hold(pg, 1)

    nav(pg, "Checklist")
    pg.wait_for_timeout(1500); capture(pg, cut=True)
    hold(pg, 3.5)

    nav(pg, "Firmware")
    pg.wait_for_timeout(3000); capture(pg, cut=True)
    hold(pg, 3.5)

    nav(pg, "Charts")
    pg.wait_for_timeout(2500); capture(pg, cut=True)
    hold(pg, 4.5)
    frames[-1][1] = 1.0
    br.close()

with open(DIR + "concat.txt", "w") as f:
    for name, d in frames:
        f.write(f"file '{name}'\nduration {d:.3f}\n")
    f.write(f"file '{frames[-1][0]}'\n")
print(len(frames), "frames,", round(sum(d for _, d in frames), 1), "s")
