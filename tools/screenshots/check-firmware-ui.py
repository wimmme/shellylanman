"""Browser test of Settings → Firmware and the status pills (DECISIONS §34, P25-1/5).

    SCRIPT=check-firmware-ui.py sh tools/screenshots/run.sh

Exit 1 when a device with a newer firmware does not get the orange ring and ⚡, the old
↑ is still there, the Firmware tab does not switch between the two modes and save, or the
phone address is still on the Network tab. Screenshots: /shots/fw-pills.png, /shots/fw-settings.png.
"""
import json
import sys
import urllib.request

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def settings():
    with urllib.request.urlopen(B + "api/v1/settings") as r:
        return json.load(r)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    # The refused https address is a 400 on purpose.
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" and "status of 400" not in m.text else None)

    # Devices: the simulated Pro3EM and S4SW say a newer stable firmware exists.
    pg.goto(B + "#/devices")
    pg.wait_for_selector(".pill.update", timeout=30000)
    upd = pg.locator(".pill.update").first
    check(upd.locator(".pill-upd").inner_text() == "⚡", "⚡ after the label")
    ring = upd.evaluate("e => getComputedStyle(e).borderTopColor")
    want = pg.evaluate("() => { const s = document.createElement('span'); s.style.color = 'var(--accent-warn)'; document.body.append(s); const c = getComputedStyle(s).color; s.remove(); return c }")
    check(ring == want, f"orange ring ({ring}, --accent-warn {want})")
    check(upd.evaluate("e => getComputedStyle(e).color") != ring, "label stays green")
    check(pg.locator(".pill", has_text="↑").count() == 0, "no ↑ left")
    upd.scroll_into_view_if_needed()
    pg.screenshot(path="/shots/fw-pills.png")

    # Firmware page: the same mark.
    pg.goto(B + "#/firmware")
    pg.wait_for_selector("table.fw .pill.update", timeout=30000)
    check(True, "Firmware page shows the mark")

    # Settings → Network no longer has the phone address; Settings → Firmware has it.
    pg.goto(B + "#/settings"); pg.wait_for_selector(".server-ports", timeout=10000)
    pg.locator("[data-tab=network]").click()
    pg.wait_for_selector("#scanRefresh", timeout=10000)
    check(pg.locator("#phoneBase").count() == 0, "phone address gone from Network")
    tabs = pg.locator(".tabs button").all_inner_texts()
    check(tabs[1:3] == ["Network", "Firmware"], f"Firmware tab after Network ({tabs})")
    pg.locator("[data-tab=firmware]").click()
    pg.wait_for_selector("input[name=fwMode]", timeout=10000)
    check(pg.locator("#phoneBase").count() == 1, "phone address on Firmware")
    check(pg.locator("input[name=fwMode][value=device]").is_checked(), "devices check themselves by default")
    check(pg.locator("#fwDeviceBase").count() == 0, "no device address in device mode")
    pg.locator("input[name=fwMode][value=local]").check()
    check(pg.locator("#fwDeviceBase").is_visible(), "device address in local mode")
    pg.locator("#fwDeviceBase").fill("https://slm.example")
    pg.get_by_role("button", name="Save").first.click()
    pg.wait_for_timeout(500)
    check(settings().get("firmware", {}).get("mode") != "local", "https address refused")
    pg.locator("#fwDeviceBase").fill("")
    pg.get_by_role("button", name="Save").first.click()
    pg.wait_for_timeout(500)
    check(settings().get("firmware", {}).get("mode") == "local", "local mode saved")
    pg.screenshot(path="/shots/fw-settings.png")

    # Firmware page in local mode: the intro says so.
    pg.goto(B + "#/firmware")
    pg.wait_for_selector("text=ShellyLanMan checks for new firmware", timeout=10000)
    check(True, "Firmware page intro in local mode")

    # Back to the default (the page reopens on the last tab).
    pg.goto(B + "#/settings"); pg.wait_for_selector("[data-tab=firmware]", timeout=10000)
    pg.locator("[data-tab=firmware]").click()
    pg.wait_for_selector("input[name=fwMode]", timeout=10000)
    pg.locator("input[name=fwMode][value=device]").check()
    pg.get_by_role("button", name="Save").first.click()
    pg.wait_for_timeout(500)
    check(settings().get("firmware", {}).get("mode") == "device", "device mode saved")
    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
