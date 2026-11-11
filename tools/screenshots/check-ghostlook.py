"""Browser test: after a (re)start in an mDNS mode, archived devices that are still
being looked for show the blue spinner on the Devices page (lookingUntil).

    SCRIPT=check-ghostlook.py sh tools/screenshots/run.sh

Switches the test instance to offline (the archive only, as after a restart), then
to the full mDNS mode with Auto reload (mDNS finds nothing on loopback, so the
simulators stay archived until their last address is tried after 45 s), and checks
the spinner; then sets the IP scan back.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
IP = {"mode": "ip", "ranges": [{"base": "127.0.0", "first": 2, "last": 14}], "refreshSeconds": 2, "configTics": 5}
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1920, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.goto(B + "#/devices")
    pg.wait_for_selector("table.devices tbody tr", timeout=30000)
    pg.wait_for_timeout(8000)  # the archive is written every 5 s

    api = ctx.request
    # Offline first: the list is the archive only, as after a restart of ShellyLanMan.
    api.put(B + "api/v1/settings", data={"scan": {**IP, "mode": "offline"}, "archive": {"use": True, "autoReload": True}})
    api.post(B + "api/v1/scan")
    pg.wait_for_timeout(2000)
    r = api.put(B + "api/v1/settings", data={"scan": {**IP, "mode": "full"}, "archive": {"use": True, "autoReload": True}})
    check(r.ok, f"full mDNS mode set ({r.status})")
    api.post(B + "api/v1/scan")
    pg.wait_for_timeout(4000)
    ghosts = pg.locator("table.devices tbody tr", has=pg.locator(".pill.ghost"))
    n = ghosts.count()
    spun = pg.locator("table.devices tbody tr:has(.pill.ghost) .spinner").count()
    check(n > 0, f"archived devices after the rescan ({n})")
    check(spun >= n and n > 0, f"they show the spinner while looked for ({spun} spinners, {n} archived)")
    pg.screenshot(path="/shots/ghost-looking.png")
    scan = api.get(B + "api/v1/scan").json()
    check(scan.get("lookingUntil", 0) > 0, f"scan state has lookingUntil ({scan.get('lookingUntil')})")

    api.put(B + "api/v1/settings", data={"scan": IP})
    api.post(B + "api/v1/scan")
    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
