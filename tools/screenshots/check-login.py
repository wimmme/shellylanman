"""Browser test of the optional UI password under the real Content-Security-Policy.

Runs in the Playwright image against ShellyLanMan with simulated devices:

    SCRIPT=check-login.py sh tools/screenshots/run.sh

Exit 1 when Settings → Security does not tick the rules and show the strength,
switching the password on does not keep this browser logged in, another
browser does not get the login page, a wrong password is not refused, the
right one with "Stay logged in" does not open the app, logging out does not
return to the login, or the browser refuses anything because of the CSP.

Screenshots: /shots/security.png, login.png, login-wrong.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
PW = "Kitchen-7-lamp!"
problems = []
csp = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def page(br):
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1280, "height": 860})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    pg.on("console", lambda m: csp.append(m.text) if "Content Security Policy" in m.text or "Refused" in m.text else None)
    return pg


with sync_playwright() as p:
    br = p.chromium.launch()

    # Settings → Security: the rules and the strength while typing.
    pg = page(br)
    pg.goto(B + "#/settings"); pg.wait_for_timeout(1500)
    pg.get_by_role("tab", name="Security").click()
    pg.wait_for_selector("#secNew")
    on = pg.get_by_role("button", name="Switch the password on")
    pg.fill("#secNew", "abc")
    rules = pg.locator(".sec-rules li.ok").count()
    check(rules == 0 and on.is_disabled(), f"weak password: no rule met ({rules}), button off")
    pg.fill("#secNew", PW)
    check(pg.locator(".sec-rules li.ok").count() == 2, "both rules met")
    check(pg.locator("#secMeter").get_attribute("value") == "4" and "strong" in pg.locator(".sec-strength").inner_text(),
          "strength shown as strong")
    pg.fill("#secAgain", PW[:-1])
    check("differ" in pg.locator(".sec-match").inner_text() and on.is_disabled(), "mismatch shown, button off")
    pg.fill("#secAgain", PW)
    check(on.is_enabled(), "button on when both match")
    pg.screenshot(path="/shots/security.png")
    on.click()
    pg.wait_for_load_state("load"); pg.wait_for_timeout(2000)
    check(pg.locator(".btn.logout").count() == 1, "still logged in after switching on, Log out shown")

    # Another browser: the login page; a wrong and the right password.
    other = page(br)
    other.goto(B); other.wait_for_selector("#loginPw", timeout=10000)
    check(other.locator(".sidebar").count() == 0, "login page instead of the app")
    other.screenshot(path="/shots/login.png")
    other.fill("#loginPw", "Wrong-pass1")
    other.get_by_role("button", name="Log in").click()
    other.locator(".login-msg", has_text="Wrong password").wait_for(timeout=10000)
    check(True, "wrong password refused")
    other.screenshot(path="/shots/login-wrong.png")
    other.fill("#loginPw", PW)
    other.check("#loginRemember")
    other.get_by_role("button", name="Log in").click()
    other.wait_for_selector(".sidebar", timeout=15000)
    cookie = [c for c in other.context.cookies() if c["name"] == "slm_session"]
    check(bool(cookie) and cookie[0]["httpOnly"] and cookie[0]["expires"] > 0, f"logged in, persistent HttpOnly cookie {cookie}")

    # Log out: back to the login.
    other.locator(".btn.logout").click()
    other.wait_for_selector("#loginPw", timeout=10000)
    check(True, "log out returns to the login")

    # Switch the password off again from the first browser.
    pg.goto(B + "#/settings"); pg.wait_for_timeout(1500)
    pg.get_by_role("tab", name="Security").click()
    pg.fill("#secCurrent", PW)
    pg.get_by_role("button", name="Switch off").click()
    pg.wait_for_load_state("load"); pg.wait_for_timeout(2000)
    check(pg.locator(".btn.logout").count() == 0, "password off, no Log out")
    br.close()

check(not csp, f"no CSP refusals {csp}")
if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
