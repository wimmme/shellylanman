"""Browser test of the profiles (Settings → Profiles) and the wizard "Set up a new Shelly"
(DECISIONS §29) under the real Content-Security-Policy.

    SCRIPT=check-provision.py sh tools/screenshots/run.sh

Exit 1 when a profile cannot be made, shown, edited or deleted, a password is shown or returned,
a validation error is not shown, the wizard does not walk through its steps, or the page logs a
CSP error. Applying a profile is tried on a simulated device through the API (nothing real is touched).

The wizard's last step waits for a new device on the network; the check cannot make one appear, so
that step is only checked up to "waiting" (the detection is covered by the unit tests).

Screenshots: /shots/prof-list.png, prof-edit.png, prov-profile.png, prov-page.png.
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

    # ---- Settings → Profiles ----
    pg.goto(B + "#/settings"); pg.wait_for_timeout(2000)
    pg.get_by_role("tab", name="Profiles").click()
    pg.get_by_text("No profiles yet.").wait_for(timeout=5000)
    check(True, "no profiles to begin with")
    pg.get_by_role("button", name="New profile").click()
    dlg = pg.locator(".modal[role=dialog]")
    dlg.wait_for()
    fields = dlg.locator(".field")
    dlg.locator("input[type=text]").first.fill("Home")
    dlg.get_by_label("Device name").fill("{model} {mac4}")
    dlg.get_by_label("Time server").fill("pool.ntp.org")
    dlg.get_by_label("Shelly Cloud").select_option("off")
    dlg.get_by_label("Login", exact=True).select_option("on")
    dlg.get_by_label("Password").first.wait_for()
    # A login needs a password: the server says so and the dialog stays.
    dlg.get_by_role("button", name="Save").click()
    dlg.locator(".sec-error").get_by_text("A login needs a password.").wait_for(timeout=5000)
    check(True, "a login without a password is refused, the message is shown")
    pg.screenshot(path="/shots/prof-edit.png")
    dlg.get_by_label("Password").first.fill("Secret-pass1")
    dlg.get_by_role("button", name="Save").click()
    pg.locator(".modal[role=dialog]").wait_for(state="detached", timeout=5000)
    item = pg.locator(".prof-item")
    check(item.count() == 1 and item.inner_text().startswith("Home"), "the profile is listed")
    txt = item.inner_text()
    check("Device name" not in txt and "Name: {model} {mac4}" in txt and "Time server: pool.ntp.org" in txt and "Cloud: Off" in txt and "Login: admin" in txt, "its settings are described")
    check("Secret-pass1" not in pg.content(), "the password is nowhere on the page")
    api = pg.evaluate("fetch('api/v1/profiles').then(r => r.text())")
    check("Secret-pass1" not in api and '"loginPasswordSet":true' in api.replace(" ", ""), "the API says a password is set, never what it is")
    pg.screenshot(path="/shots/prof-list.png")

    # Edit: the password is not filled in; saving without typing keeps it.
    item.get_by_role("button", name="Edit").click()
    dlg = pg.locator(".modal[role=dialog]")
    dlg.wait_for()
    check(dlg.get_by_label("Password").first.input_value() == "", "the stored password is not filled in")
    check(dlg.get_by_text("A password is stored").count() == 1, "it says one is stored")
    dlg.locator("input[type=text]").first.fill("Home 2")
    dlg.get_by_role("button", name="Save").click()
    pg.locator(".modal[role=dialog]").wait_for(state="detached", timeout=5000)
    check(pg.locator(".prof-item strong").inner_text() == "Home 2", "renamed, password kept")
    api = pg.evaluate("fetch('api/v1/profiles').then(r => r.json())")
    check(api[0]["loginPasswordSet"] is True, "the password survived the edit")
    pid = api[0]["id"]

    # ---- the wizard ----
    pg.goto(B + "#/devices"); pg.wait_for_timeout(2500)
    pg.get_by_role("button", name="Set up a new Shelly…").click()
    wz = pg.locator(".modal[role=dialog]")
    wz.wait_for()
    check(wz.locator(".apw-steps li").count() == 4, "four steps")
    check(wz.locator("select").input_value() == pid and wz.get_by_text("Time server: pool.ntp.org").count() == 1, "the profile is chosen and summarised")
    pg.screenshot(path="/shots/prov-profile.png")
    nxt = wz.get_by_role("button", name="Next")
    nxt.click()
    wz.get_by_text("starts with Shelly").wait_for(timeout=5000)
    check(wz.locator("img.lfw-qr").count() == 0, "join without a name: the text, no QR code")
    wz.get_by_role("button", name="Back").click()
    wz.locator("input[type=text]").fill("ShellyPlus1-A8032AB636EC")
    wz.locator("input[type=text]").press("Tab")
    wz.get_by_text("Recognised").wait_for(timeout=5000)
    nxt.click()
    wz.locator("img.lfw-qr").wait_for(timeout=5000)
    check(wz.get_by_text("ShellyPlus1-A8032AB636EC").count() >= 1 and wz.get_by_text("The network is open").count() == 1, "join with a name: the name and the QR code")
    nxt.click()
    wz.locator("img.lfw-qr").wait_for(timeout=5000)
    check(wz.get_by_text("http://192.168.33.1").count() >= 1 and wz.get_by_text("never stores it").count() == 1, "page: the address, its QR code, and the password note")
    pg.screenshot(path="/shots/prov-page.png")
    nxt.click()
    wz.locator(".apw-steps li.current").get_by_text("Set up").wait_for()
    wz.get_by_text("Waiting for the new device").wait_for(timeout=10000)
    check(wz.get_by_text("Waiting for the new device").count() == 1 and not nxt.is_visible(), "the last step waits for a new device")
    pg.keyboard.press("Escape")
    check(pg.locator(".modal[role=dialog]").count() == 0, "Escape closes the wizard")

    # ---- applying, on a simulated device ----
    dev = pg.evaluate("fetch('api/v1/devices').then(r => r.json()).then(l => l.find(d => d.gen === '1' && d.status === 'online')?.id)")
    check(bool(dev), f"a simulated Gen1 device ({dev})")
    plan = pg.evaluate("(a) => fetch('api/v1/profiles/' + a[0] + '/plan?device=' + a[1]).then(r => r.json())", [pid, dev])
    check([s["step"] for s in plan] == ["name", "ntp", "cloud", "login"], f"the plan lists the steps in order {plan}")
    r = pg.evaluate("""(a) => fetch('api/v1/profiles/' + a[0] + '/apply', {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({device: a[1], confirm: true})}).then(async r => [r.status, await r.json()])""", [pid, dev])
    check(r[0] == 200 and {s["step"]: s["result"] for s in r[1]["steps"]}.get("name") == "ok", f"applied to a simulator {r}")
    no = pg.evaluate("""(a) => fetch('api/v1/profiles/' + a[0] + '/apply', {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({device: a[1]})}).then(r => r.status)""", [pid, dev])
    check(no == 428, "without confirm: 428")

    # ---- delete ----
    pg.goto(B + "#/settings"); pg.wait_for_timeout(1500)
    pg.get_by_role("tab", name="Profiles").click()
    pg.get_by_role("button", name="Delete").first.click()
    pg.locator(".modal[role=dialog]").get_by_role("button", name="Delete").click()
    pg.get_by_text("No profiles yet.").wait_for(timeout=5000)
    check(True, "the profile is deleted")
    check(not [e for e in errors if "Content Security Policy" in e], f"no CSP errors {errors}")

    ph = ctx.new_page()
    ph.set_viewport_size({"width": 390, "height": 844})
    ph.goto(B + "#/settings"); ph.wait_for_timeout(1500)
    ph.get_by_role("tab", name="Profiles").click()
    ph.get_by_role("button", name="New profile").click()
    ph.locator(".modal[role=dialog]").wait_for()
    check(ph.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "phone: no sideways scroll")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
