"""Browser test of Settings → General → Ports (DECISIONS P17-1).

    SCRIPT=check-ports.py sh tools/screenshots/run.sh

Exit 1 when the ports table is missing, the port is not 3082 with "The default",
or anything still offers to change the port. Screenshot: /shots/ports.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    pg.goto(B + "#/settings"); pg.wait_for_selector(".server-ports", timeout=10000)
    text = pg.locator(".server-ports").inner_text()
    check("3082" in text and "The default" in text and "SHELLYLANMAN_PORT" in text, f"port, source and how to change: {text!r}")
    check(pg.locator("#srvPort").count() == 0 and pg.get_by_role("button", name="Change port").count() == 0, "no way to change the port here")
    check("bridge mode" in text, "bridge-mode note outside Home Assistant")
    pg.locator(".server-ports").scroll_into_view_if_needed()
    pg.screenshot(path="/shots/ports.png")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
