"""Browser test of the firmware wizard that works through a device's own access point
(DECISIONS §29) under the real Content-Security-Policy.

    SCRIPT=check-apwizard.py sh tools/screenshots/run.sh

Exit 1 when the wizard does not open from the Firmware page, does not recognise an access
point name, does not ask for the model of an unknown name, a step does not show its QR code or
text, the device is not found by its MAC, or the page logs a CSP error.

The firmware step asks Shelly's firmware index over the internet; the check accepts the QR code
or the banner that says the index could not be read, and tests every other step strictly.

Screenshots: /shots/apw-which.png, apw-join.png, apw-page.png.
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
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.goto(B + "#/firmware"); pg.wait_for_timeout(2500)

    pg.get_by_role("button", name="Through an access point…").click()
    dlg = pg.locator(".modal[role=dialog]")
    dlg.wait_for(timeout=5000)
    check(dlg.locator(".apw-steps li").count() == 5, "five steps")
    check(dlg.locator(".apw-steps li.current").inner_text().endswith("Device"), "starts at Device")
    nxt = dlg.get_by_role("button", name="Next")
    check(nxt.is_disabled(), "Next waits for a model")

    # An access point name that says what it is.
    name = dlg.locator("input[type=text]")
    name.fill("ShellyPlugSG3-54320467CBD4")
    dlg.get_by_role("button", name="Recognise").click()
    dlg.get_by_text("Plug S G3").wait_for(timeout=5000)
    check(dlg.get_by_text("54320467CBD4").count() >= 1, "recognised: model and MAC")
    check(nxt.is_enabled(), "Next is on")
    pg.screenshot(path="/shots/apw-which.png")

    # A name that does not: the user picks the model.
    name.fill("MyShellyThing")
    dlg.get_by_role("button", name="Recognise").click()
    sel = dlg.locator(".apw-result select")
    sel.wait_for(timeout=5000)
    check(nxt.is_disabled() and sel.locator("option").count() > 60, "unknown name: pick a model, Next waits")
    sel.select_option(label="PlugS")
    check(nxt.is_enabled(), "a picked model lets it go on")

    # A device of the list, by its MAC.
    mac = pg.evaluate("fetch('api/v1/devices').then(r => r.json()).then(l => l.find(d => d.gen === '1' || d.gen === '2' || d.gen === '3')?.id)")
    check(bool(mac), f"the simulators gave a device ({mac})")
    name.fill(mac)
    dlg.get_by_role("button", name="Recognise").click()
    dlg.get_by_text("This device is in your list").wait_for(timeout=5000)
    check(True, "a MAC of a device in the list finds it")

    # Back to the recognised name and through the steps.
    name.fill("ShellyPlugSG3-54320467CBD4")
    dlg.get_by_role("button", name="Recognise").click()
    dlg.get_by_text("Plug S G3").wait_for(timeout=5000)
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Firmware").wait_for()
    pg.wait_for_timeout(4000)
    got = dlg.locator("img.lfw-qr").count() == 1 or dlg.locator(".banner.warn").count() == 1
    check(got, "firmware step: the QR code, or the banner that the index could not be read")
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Join").wait_for()
    dlg.locator("img.lfw-qr").wait_for(timeout=5000)
    check(dlg.get_by_text("ShellyPlugSG3-54320467CBD4").count() >= 1 and dlg.get_by_text("The network is open").count() == 1, "join step: the name, the QR code, open")
    check(dlg.get_by_text("Switch it on").count() == 0, "a name alone has no access point to switch on")
    pg.screenshot(path="/shots/apw-join.png")
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Open page").wait_for()
    dlg.locator("img.lfw-qr").wait_for(timeout=5000)
    check(dlg.get_by_text("http://192.168.33.1").count() >= 1, "page step: the address and its QR code")
    pg.screenshot(path="/shots/apw-page.png")
    nxt.click()
    dlg.locator(".apw-steps li.current").get_by_text("Update").wait_for()
    check("Waiting for" in dlg.locator("p[role=status]").inner_text(), "wait step: waiting for the device")
    check(not nxt.is_visible(), "no Next after the last step")
    dlg.get_by_role("button", name="Back").click()
    dlg.locator(".apw-steps li.current").get_by_text("Open page").wait_for()
    check(True, "Back goes back")

    pg.keyboard.press("Escape")
    check(pg.locator(".modal[role=dialog]").count() == 0, "Escape closes the wizard")
    check(not [e for e in errors if "Content Security Policy" in e], f"no CSP errors {errors}")

    ph = ctx.new_page()
    ph.set_viewport_size({"width": 390, "height": 844})
    ph.goto(B + "#/firmware"); ph.wait_for_timeout(2000)
    ph.get_by_role("button", name="Through an access point…").click()
    ph.locator(".modal[role=dialog]").wait_for()
    check(ph.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "phone: no sideways scroll")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
