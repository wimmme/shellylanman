"""Browser test of the address check in Settings → Firmware (DECISIONS P32-1).

    SCRIPT=check-addrcheck.py sh tools/screenshots/run.sh

Exit 1 when the address for phones is not checked after saving: ShellyLanMan's own
address must say it answers, a simulator's (something else, as Home Assistant on 8123)
must warn with ShellyLanMan's own port, a closed port must say it could not be reached.

Screenshot: /shots/addrcheck.png (the warning).
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
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1400, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)

    pg.goto(B + "#/settings"); pg.wait_for_timeout(1500)
    pg.get_by_role("tab", name="Firmware").click()
    pg.wait_for_selector("#phoneBase")
    help_text = pg.locator("#phoneBase ~ small").inner_text()
    check("wizard" in help_text and "8123" in help_text, f"the help names the wizard and Home Assistant's port: {help_text!r}")
    note = pg.locator(".addr-note[data-for=phone]")
    save = pg.get_by_role("button", name="Save").nth(1)

    def try_address(url, sel):
        pg.fill("#phoneBase", url)
        save.click()
        try:
            pg.wait_for_selector(".addr-note[data-for=phone] " + sel, timeout=10000)
            return note.inner_text()
        except Exception:
            return None

    text = try_address("http://127.0.0.1:3082", ".addr-ok")
    check(text is not None and "answers" in text, f"own address: ok ({text!r})")

    text = try_address("http://127.0.0.2", ".addr-other")  # a simulated Shelly: something else answers
    check(text is not None and "3082" in text, f"something else: warning with ShellyLanMan's port ({text!r})")
    pg.screenshot(path="/shots/addrcheck.png")

    text = try_address("http://127.0.0.1:1", ".addr-unreachable")
    check(text is not None and "could not reach" in text, f"closed port: not reachable ({text!r})")

    # Reopened, the page checks the saved address at once.
    pg.reload(); pg.wait_for_timeout(1000)
    pg.get_by_role("tab", name="Firmware").click()
    try:
        pg.wait_for_selector(".addr-note[data-for=phone] .addr-unreachable", timeout=10000)
        check(True, "checked again when the page opens")
    except Exception:
        check(False, "checked again when the page opens")

    # Empty: no note.
    pg.fill("#phoneBase", ""); save.click(); pg.wait_for_timeout(1500)
    check(note.inner_text().strip() == "", f"empty address: no note ({note.inner_text()!r})")

    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
