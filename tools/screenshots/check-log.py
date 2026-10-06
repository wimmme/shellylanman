"""Browser test of the Log page (ShellyLanMan's own log) under the real CSP.

    SCRIPT=check-log.py sh tools/screenshots/run.sh

Exit 1 when the page is not in the sidebar directly above Settings, the start-up
lines are missing, new lines do not arrive live, the level filter, search,
pause, clear or copy do not work.

The lines that arrive live are made by setting and removing the UI password
through the API (an info and a warning line); the password is off again at the end.

Screenshots: /shots/log.png, log-phone.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def put_password(pg, body):
    return pg.evaluate(
        """async (body) => (await fetch('api/v1/auth/password', {method: 'PUT',
            headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)})).status""", body)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    ctx.grant_permissions(["clipboard-read", "clipboard-write"], origin="http://127.0.0.1:3082")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(2500)

    names = pg.eval_on_selector_all(".nav a", "els => els.map(e => e.getAttribute('href'))")
    check(names[names.index("#/settings") - 1] == "#/log", f"Log is right above Settings {names}")
    pg.click(".nav a[href='#/log']"); pg.wait_for_timeout(1500)
    check(pg.locator("h1").text_content() == "Log", "page title")

    lines = pg.locator(".logline")
    check(lines.count() > 0, f"{lines.count()} lines from the start")
    check(pg.locator(".logline", has_text="starting ShellyLanMan").count() == 1, "the start-up line is there")
    check(pg.locator(".logline .logtime").first.get_attribute("title").startswith("20"), "date in the time's tooltip")
    check(pg.locator(".log-tools .muted").inner_text().endswith("lines"), "line count shown")

    # Live: an info and a warning line.
    before = lines.count()
    check(put_password(pg, {"password": "Abcdefgh1"}) == 200, "password set through the API")
    pg.locator(".logline", has_text="UI password set").wait_for(timeout=10000)
    check(lines.count() == before + 1, "an info line arrived live")
    check(put_password(pg, {"current": "Abcdefgh1", "password": ""}) == 200, "password switched off again")
    warn = pg.locator(".logline.lvl-warn", has_text="UI password switched off")
    warn.wait_for(timeout=10000)
    check(warn.locator(".loglevel").inner_text() == "WARN", "a warning line arrived live")
    pg.screenshot(path="/shots/log.png")

    # Level filter.
    pg.select_option(".log-tools select", "warn")
    check(pg.locator(".logline:not(.lvl-warn):not(.lvl-error)").count() == 0 and warn.count() == 1, "warnings and errors only")
    pg.select_option(".log-tools select", "error")
    check(pg.locator(".logline").count() == 0 and pg.locator(".state").is_visible(), "errors only: none, the empty message")
    pg.select_option(".log-tools select", "info")
    check(lines.count() > before, "information and up: everything again")

    # Search.
    pg.fill(".log-tools input[type=search]", "switched off")
    check(lines.count() == 1, "search finds the one line")
    pg.fill(".log-tools input[type=search]", "no such line anywhere")
    check(lines.count() == 0, "search without a match")
    pg.fill(".log-tools input[type=search]", "")

    # Pause: new lines wait, then appear.
    pg.get_by_role("button", name="Pause").click()
    n = lines.count()
    put_password(pg, {"password": "Abcdefgh1"})
    pg.wait_for_timeout(1500)
    check(lines.count() == n, "paused: no new line")
    pg.get_by_role("button", name="Resume").click()
    check(lines.count() == n + 1, "resumed: the line that waited is there")
    put_password(pg, {"current": "Abcdefgh1", "password": ""})
    pg.wait_for_timeout(1000)

    # Copy.
    pg.get_by_role("button", name="Copy").click()
    clip = pg.evaluate("navigator.clipboard.readText()")
    check("WARN UI password switched off" in clip and clip.splitlines()[0][:2] == "20", "copy: lines with date")

    # Clear hides, a new line still comes.
    pg.get_by_role("button", name="Clear").click()
    check(lines.count() == 0, "cleared")
    put_password(pg, {"password": "Abcdefgh1"})
    pg.locator(".logline", has_text="UI password set").wait_for(timeout=10000)
    check(lines.count() == 1, "after clear only the new line")
    put_password(pg, {"current": "Abcdefgh1", "password": ""})

    # The page keeps working after leaving and coming back (no double lines, subscription dropped).
    pg.click(".nav a[href='#/devices']"); pg.wait_for_timeout(500)
    pg.click(".nav a[href='#/log']"); pg.wait_for_timeout(1500)
    check(pg.locator(".logline", has_text="starting ShellyLanMan").count() == 1, "re-opened: the start-up line once")
    check(not [e for e in errors if "Content Security Policy" in e], f"no CSP errors {errors}")

    ph = ctx.new_page()
    ph.set_viewport_size({"width": 390, "height": 844})
    ph.goto(B + "#/log"); ph.wait_for_timeout(2000)
    check(ph.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "phone: no sideways scroll")
    ph.screenshot(path="/shots/log-phone.png")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
