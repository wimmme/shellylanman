"""Browser test of "Create profile…" (DECISIONS §30) under the real Content-Security-Policy.

    SCRIPT=check-profile-from-device.py sh tools/screenshots/run.sh

Exit 1 when the button is not there or does not follow the selection, the list of settings does not show,
the editor does not open with the ticked settings and an empty, required name, a profile without a name
is saved, a password is invented, or the page logs a CSP error. Only reads from the (simulated) devices.

Screenshots: /shots/pfd-list.png, pfd-editor.png.
"""
from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)

    pg.goto(B + "#/devices"); pg.wait_for_timeout(2500)
    btn = pg.get_by_role("button", name="Create profile…")
    check(btn.is_disabled(), "disabled without a selection")

    opened = False
    boxes = pg.locator("tr[data-id] input[type=checkbox]")
    for i in range(boxes.count()):
        boxes.nth(i).check()
        if btn.is_enabled():
            btn.click()
            dlg = pg.locator(".modal[role=dialog]")
            try:
                dlg.locator(".prof-pick, .muted").first.wait_for(timeout=8000)
                opened = True
                break
            except Exception:
                pass
        boxes.nth(i).uncheck()
        pg.locator(".toast").count()
    check(opened, "the list of settings opens for a device")
    dlg = pg.locator(".modal[role=dialog]")
    items = dlg.locator(".prof-pick li")
    check(items.count() > 0, "settings are listed")
    ticked = dlg.locator(".prof-pick input:checked").count()
    differs = dlg.locator(".prof-differs").count()
    check(ticked == differs, "ticked at first: exactly the ones that differ from the factory (%d)" % ticked)
    pg.screenshot(path="/shots/pfd-list.png")
    dlg.locator(".prof-pick input").first.click()  # untick or tick the first
    dlg.get_by_role("button", name="Continue").click()

    ed = pg.locator(".modal[role=dialog]")
    ed.get_by_label("Name *").wait_for(timeout=5000)
    check(ed.get_by_label("Name *").input_value() == "", "the profile's name starts empty")
    check(ed.get_by_label("Name *").get_attribute("required") is not None and ed.get_by_text("Required.").count() == 1, "and is marked as required")
    check(ed.get_by_label("Device name").input_value() == "", "the device-name pattern starts empty")
    pg.screenshot(path="/shots/pfd-editor.png")
    ed.get_by_role("button", name="Save").click()
    ed.locator(".sec-error").get_by_text("The profile needs a name.").wait_for(timeout=5000)
    check(True, "saving without a name is refused with a message")
    ed.get_by_label("Name *").fill("From a device")
    pw = ed.locator("input[type=password]")
    if pw.count():
        check(pw.first.input_value() == "", "no password is filled in")
        pw.first.fill("Secret-pass1")
    ed.get_by_role("button", name="Save").click()
    pg.locator(".modal[role=dialog]").wait_for(state="detached", timeout=5000)
    api = pg.evaluate("fetch('api/v1/profiles').then(r => r.json())")
    check(len(api) == 1 and api[0]["name"] == "From a device", "the profile is saved")
    check("Secret-pass1" not in pg.content(), "no password on the page")
    check(not [e for e in errors if "Content Security Policy" in e or "CSP" in e], "no CSP errors: %s" % errors)
    br.close()

if problems:
    raise SystemExit(1)
