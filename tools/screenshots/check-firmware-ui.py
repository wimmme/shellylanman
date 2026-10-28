"""Browser test of Settings → Firmware and the status pills (DECISIONS §34, P25-1/5).

    SCRIPT=check-firmware-ui.py sh tools/screenshots/run.sh

Exit 1 when a device with a newer firmware does not get the orange ring and ⚡, the old
↑ is still there, the Firmware tab does not switch between the two modes and save, or the
phone address is still on the Network tab; when the status pills of Firmware and Checklist have
no hover text, a version has no copy button, the Archive tab does not hold the backups setting
with its boxes in front of the text, or a chart brings back a series hidden from its legend or
does not move again after a pause. Screenshots: /shots/fw-pills.png, /shots/fw-settings.png,
/shots/fw-copy.png, /shots/archive.png, /shots/chart-hidden.png.
"""
import json
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def until(cond, seconds=60):
    """Waits for cond (wait_for_function evaluates a string: refused by the page's CSP)."""
    end = time.time() + seconds
    while not cond():
        if time.time() > end:
            raise TimeoutError("waited too long")
        time.sleep(0.25)


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

    tip = upd.locator("xpath=ancestor::td").get_attribute("title") or ""
    check("update" in tip.lower(), f"Devices: the pill's hover text ({tip})")

    # Firmware page: the same mark, the same hover text, a copy button after a version.
    pg.goto(B + "#/firmware")
    pg.wait_for_selector("table.fw .pill.update", timeout=30000)
    check(True, "Firmware page shows the mark")
    # While a row is read, its spinner stands after the status, not after the name.
    spin = pg.locator("table.fw tbody .spinner")
    if spin.count():
        check(pg.locator("table.fw tbody td:nth-child(3) .spinner").count() == 0, "Firmware: the spinner after the status")
    until(lambda: pg.locator("table.fw .spinner").count() == 0)
    ftip = pg.locator("table.fw .pill.update").first.locator("xpath=ancestor::td").get_attribute("title") or ""
    check("update" in ftip.lower(), f"Firmware: the pill's hover text ({ftip})")
    copy = pg.locator("table.fw button.fw-link")
    check(copy.count() >= 1 and copy.first.get_attribute("title") == "Copy download link", f"Firmware: copy buttons ({copy.count()})")
    if copy.count():
        copy.first.click()
        # Shelly's index is asked over the internet: a link copied, or the reason it was not.
        pg.wait_for_selector(".toast", timeout=60000)
        check(True, "Firmware: the copy button answers (" + pg.locator(".toast").first.inner_text() + ")")
    pg.screenshot(path="/shots/fw-copy.png")

    # Checklist: the hover text too.
    pg.goto(B + "#/checklist")
    until(lambda: pg.locator("table.checklist .spinner").count() == 0 and pg.locator("table.checklist tbody tr").count() > 0)
    ctip = pg.locator("table.checklist tbody tr td:nth-child(2)").first.get_attribute("title") or ""
    check(ctip != "", f"Checklist: the pill's hover text ({ctip})")

    # Settings → Network no longer has the phone address; Settings → Firmware has it.
    pg.goto(B + "#/settings"); pg.wait_for_selector(".server-ports", timeout=10000)
    pg.locator("[data-tab=network]").click()
    pg.wait_for_selector("#scanRefresh", timeout=10000)
    check(pg.locator("#phoneBase").count() == 0, "phone address gone from Network")
    check(pg.locator("#backupKeep").count() == 0, "backups gone from Network")
    pg.locator("[data-tab=archive]").click()
    pg.wait_for_selector("#backupKeep", timeout=10000)
    for line in pg.locator("label.check-line").all():
        box, text = line.locator("input").bounding_box(), line.bounding_box()
        check(box["x"] - text["x"] < 8 and box["y"] - text["y"] < 8, f"Archive: the box in front of its text ({box}, {text})")
    keep = pg.locator("#backupKeep").bounding_box()
    clear = pg.get_by_role("button", name="Clear archive").bounding_box()
    check(keep["y"] > clear["y"], "Archive: the backups setting under Save / Clear archive")
    red = pg.get_by_role("button", name="Clear archive")
    classes = red.evaluate("e => [e.previousElementSibling.className, e.className]")
    check(classes == ["btn", "btn danger-text"], f"Archive: grey buttons, Clear archive in red letters ({classes})")
    colours = red.evaluate("e => [getComputedStyle(e).color, getComputedStyle(e.previousElementSibling).color, getComputedStyle(e).backgroundColor, getComputedStyle(e.previousElementSibling).backgroundColor]")
    check(colours[0] != colours[1] and colours[2] == colours[3], f"Archive: red letters on the same grey ({colours})")
    pg.screenshot(path="/shots/archive.png")
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

    # Charts: a series hidden from the legend stays hidden; pause holds the chart, resume moves it again.
    dev = pg.evaluate("fetch('api/v1/devices').then(r => r.json()).then(l => l.find(d => d.status === 'online' && (d.meters || []).length)?.id)")
    pg.goto(B + "#/charts?ids=" + dev)
    canvas = pg.locator(".chart-box canvas")
    canvas.wait_for(timeout=10000)
    pg.wait_for_timeout(4000)
    shot = lambda: canvas.evaluate("c => c.toDataURL()")
    # The legend box of the first series: its colour (#38bdf8) near the bottom of the canvas.
    blue = "Math.abs(d[i] - 56) < 12 && Math.abs(d[i+1] - 189) < 12 && Math.abs(d[i+2] - 248) < 12"
    spot = canvas.evaluate("c => { const x = c.getContext('2d'), w = c.width, h = c.height, d = x.getImageData(0, h - 60, w, 60).data;"
                           " for (let i = 0; i < d.length; i += 4) if (" + blue + ") return [(i / 4) % w, h - 60 + Math.floor(i / 4 / w), w / c.clientWidth]; return null }")
    line = ("c => { const x = c.getContext('2d'), d = x.getImageData(0, 0, c.width, c.height - 60).data; let n = 0;"
            " for (let i = 0; i < d.length; i += 4) if (" + blue + ") n++; return n }")
    check(spot is not None and canvas.evaluate(line) > 0, f"Charts: a first series with a legend box ({spot})")
    if spot:
        canvas.click(position={"x": spot[0] / spot[2] + 3, "y": spot[1] / spot[2] + 3})
        pg.wait_for_timeout(6000)  # several readings later
        check(canvas.evaluate(line) == 0, "Charts: a series hidden from the legend stays hidden")
        pg.screenshot(path="/shots/chart-hidden.png")
        canvas.click(position={"x": spot[0] / spot[2] + 3, "y": spot[1] / spot[2] + 3})
        pg.wait_for_timeout(500)
        check(canvas.evaluate(line) > 0, "Charts: and comes back with a second click")
    z = shot()
    pg.wait_for_timeout(5000)
    check(shot() != z, "Charts: live, the chart moves")
    pause = pg.locator(".chart-toolbar button[aria-pressed]").nth(1)
    pause.click()
    a = shot()
    pg.wait_for_timeout(5000)
    check(shot() == a, "Charts: paused, the chart holds still")
    pause.click()
    b = shot()
    pg.wait_for_timeout(5000)
    check(shot() != b, "Charts: resumed, the chart moves again")

    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
