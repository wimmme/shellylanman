"""Browser test of the AP wizard's last step as a phone lives it (DECISIONS P25-9, P25-12):

    SCRIPT=check-apwait.py sh tools/screenshots/run.sh

The phone goes off line (it joins the device's access point) before the last step, the device
restarts meanwhile, and the phone comes back to the home network. The step must then say the
device is back, without a reload. The restart is played by answering the device list with a
short uptime for the device once the phone is back.

Exit 1 when the step does not report the device within 30 s of the phone coming back.
Screenshot: /shots/apw-wait.png.
"""
import json
import sys
import time

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 390, "height": 844})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    dev = pg.evaluate("fetch('api/v1/devices').then(r => r.json()).then(l => l.find(d => d.status === 'online' && (d.gen === '2' || d.gen === '3') && d.uptime > 100))")
    check(dev is not None, f"a Gen2+ simulator on line with an uptime ({dev and dev['id']}, {dev and dev['uptime']})")
    mac = dev["id"]

    # After the restart the device list says so: a short uptime.
    restarted = {"on": False}

    def devices(route):
        resp = route.fetch()
        body = resp.json()
        if restarted["on"]:
            for d in body:
                if d["id"] == mac:
                    d["uptime"] = 2
        route.fulfill(response=resp, body=json.dumps(body))
    pg.route("**/api/v1/devices", devices)

    pg.goto(B + "#/firmware?apw=" + mac)
    dlg = pg.locator(".modal[role=dialog]")
    dlg.get_by_text("This device is in your list").wait_for(timeout=10000)
    nxt = dlg.get_by_role("button", name="Next")
    for name in ["Firmware", "Join", "Open page"]:
        nxt.click()
        dlg.locator(".apw-steps li.current").get_by_text(name).wait_for(timeout=20000)

    # Step 4: the phone joins the device's access point (off line), the device updates and restarts.
    ctx.set_offline(True)
    pg.wait_for_timeout(3000)
    restarted["on"] = True
    # Still on step 4 when the phone is back on the home Wi-Fi; then Next.
    ctx.set_offline(False)
    pg.wait_for_timeout(1500)
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Update").wait_for(timeout=5000)
    check(dlg.get_by_role("button", name="Look again").count() == 1, "a Look again button while waiting")
    t0 = time.time()
    try:
        dlg.locator("p[role=status]", has_text="is back").wait_for(timeout=30000)
        check(True, f"the last step reports the device back ({time.time() - t0:.1f} s)")
        check(dlg.get_by_role("button", name="Look again").count() == 0, "no Look again once back")
        check(dlg.locator(".apw-content .spinner").count() == 0, "no spinner once back")
    except Exception:
        check(False, "the last step reports the device back within 30 s: " + dlg.locator("p[role=status]").inner_text())
    pg.screenshot(path="/shots/apw-wait.png")

    # The other order: on step 5 already while off line, back after the restart.
    restarted["on"] = False
    pg.keyboard.press("Escape")
    pg.goto(B + "#/devices"); pg.reload(); pg.wait_for_timeout(2500)  # forget the restarted list
    pg.goto(B + "#/firmware?apw=" + mac)
    dlg.get_by_text("This device is in your list").wait_for(timeout=10000)
    for name in ["Firmware", "Join", "Open page"]:
        nxt.click()
        dlg.locator(".apw-steps li.current").get_by_text(name).wait_for(timeout=20000)
    ctx.set_offline(True)
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Update").wait_for(timeout=5000)
    pg.wait_for_timeout(5000)
    check(dlg.locator("p[role=status]", has_text="is back").count() == 0, "off line, not back yet")
    restarted["on"] = True
    ctx.set_offline(False)
    try:  # gone already when the device was seen back first
        dlg.get_by_role("button", name="Look again").click(timeout=2000)
    except Exception:
        pass
    t0 = time.time()
    try:
        dlg.locator("p[role=status]", has_text="is back").wait_for(timeout=30000)
        check(True, f"on the last step while off line: back once on line ({time.time() - t0:.1f} s)")
        check(dlg.locator(".apw-content .spinner").count() == 0, "no spinner once back")
    except Exception:
        check(False, "on the last step while off line: back within 30 s: " + dlg.locator("p[role=status]").inner_text())
    # Wim's order: back on the home Wi-Fi, list read again, then the last step: back at once.
    pg.keyboard.press("Escape")
    pg.goto(B + "#/firmware?apw=" + mac)
    dlg.get_by_text("This device is in your list").wait_for(timeout=10000)
    for name in ["Firmware", "Join", "Open page"]:
        nxt.click()
        dlg.locator(".apw-steps li.current").get_by_text(name).wait_for(timeout=20000)
    pg.evaluate("fetch('api/v1/devices')")
    pg.wait_for_timeout(5000)
    nxt.click()
    dlg.locator("p[role=status]", has_text="is back").wait_for(timeout=10000)
    check(dlg.locator(".apw-content .spinner").count() == 0 and dlg.get_by_role("button", name="Look again").count() == 0,
          "back at once when the last step shows: no spinner, no button")
    pg.screenshot(path="/shots/apw-wait-back.png")
    check(not [e for e in errors if "Content Security Policy" in e], f"no CSP errors {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
