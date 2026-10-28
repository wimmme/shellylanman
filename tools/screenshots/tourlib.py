"""Recording helpers for the README animations (tour.py, tour-apw.py).

A recording is a series of PNGs with how long each one shows (an ffmpeg concat
list); video.sh turns it into an MP4 and a GIF. Waits for loading are cut out.
"""
import os
import time

B = "http://127.0.0.1:3082/"
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


class Recorder:
    def __init__(self, pg, name):
        self.pg = pg
        self.dir = f"/shots/{name}/"
        os.makedirs(self.dir, exist_ok=True)
        os.chmod(self.dir, 0o777)  # created as root: the next tools/remote.sh run must be able to remove it
        for f in os.listdir(self.dir):
            os.remove(self.dir + f)
        self.frames = []  # [file, seconds]
        self.last = None
        self.mouse = [W / 2, H / 2]
        pg.mouse.move(*self.mouse)

    def capture(self, cut=False):
        """Takes a frame; the previous one lasts until now (or briefly, after a cut)."""
        now = time.monotonic()
        if self.frames:
            self.frames[-1][1] = 0.1 if cut else now - self.last
        name = f"{len(self.frames):04d}.png"
        self.pg.screenshot(path=self.dir + name)
        self.frames.append([name, 0.1])
        self.last = time.monotonic() if cut else now

    def hold(self, seconds):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            self.capture()

    def wait(self, ms):
        """Waits off the record (loading) and starts again with a fresh frame."""
        self.pg.wait_for_timeout(ms)
        self.capture(cut=True)

    def move(self, loc, steps=10):
        if not self.visible(loc):
            loc.scroll_into_view_if_needed()
            self.capture()
        box = loc.bounding_box()
        self.move_to(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2, steps)

    def visible(self, loc):
        """Inside the window and not clipped by a scrolled container (a wide table)."""
        return loc.evaluate("""e => { const r = e.getBoundingClientRect(); const x = r.left + r.width / 2, y = r.top + r.height / 2;
          const at = document.elementFromPoint(x, y); return !!at && (at === e || e.contains(at)); }""")

    def move_to(self, x, y, steps=10):
        x0, y0 = self.mouse
        for i in range(1, steps + 1):
            f = i / steps
            f = f * f * (3 - 2 * f)
            self.pg.mouse.move(x0 + (x - x0) * f, y0 + (y - y0) * f)
            self.capture()
        self.mouse[:] = [x, y]

    def click(self, loc, button="left"):
        self.move(loc)
        self.pg.mouse.down(button=button); self.capture()
        self.pg.mouse.up(button=button); self.capture()

    def nav(self, label):
        self.click(self.pg.locator(".nav a", has_text=label))

    def scroll(self, selector, top, seconds=1.0):
        self.pg.evaluate("([s, t]) => document.querySelector(s).scrollTo({ top: t, behavior: 'smooth' })", [selector, top])
        self.hold(seconds)

    def save(self, tail=1.0):
        self.frames[-1][1] = tail
        with open(self.dir + "concat.txt", "w") as f:
            for name, d in self.frames:
                f.write(f"file '{name}'\nduration {d:.3f}\n")
            f.write(f"file '{self.frames[-1][0]}'\n")
        print(len(self.frames), "frames,", round(sum(d for _, d in self.frames), 1), "s")


def context(browser):
    ctx = browser.new_context(color_scheme="dark", viewport={"width": W, "height": H})
    for s in (QUIET, COLS, POINTER):
        ctx.add_init_script(s)
    return ctx
