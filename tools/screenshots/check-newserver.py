"""Browser test: a page open while ShellyLanMan is updated asks to be loaded again (DECISIONS P22-2).

    SCRIPT=check-newserver.py sh tools/screenshots/run.sh

The WebSocket's hello is changed on its way to the page, as if a newer server answered after a
reconnect. Exit 1 when no banner with a Reload button appears, when the page reloads by itself
(a script being edited would be lost), when the banner shows without a change, or on a CSP error.

Screenshot: /shots/newserver.png.
"""
import json

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def relay(newer):
    """Pass the socket through; with newer set, the hello names another version."""
    def handler(ws):
        server = ws.connect_to_server()

        def to_page(msg):
            if newer:
                try:
                    ev = json.loads(msg)
                    if ev.get("type") == "hello":
                        ev["data"]["version"] = "99.0.0"
                        msg = json.dumps(ev)
                except (ValueError, TypeError, AttributeError):
                    pass
            try:
                ws.send(msg)
            except Exception:  # the page is gone (closed at the end of the test)
                pass

        server.on_message(to_page)
        ws.on_message(lambda m: server.send(m))
    return handler


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1280, "height": 800})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    errors = []

    pg = ctx.new_page()
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.route_web_socket("**/ws", relay(False))
    pg.goto(B + "#/devices"); pg.wait_for_timeout(2500)
    check(pg.locator(".banner[role=alert]").count() == 0, "same version: no banner")
    pg.close()

    pg = ctx.new_page()
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.route_web_socket("**/ws", relay(True))
    pg.goto(B + "#/devices")
    pg.evaluate("window.__marker = 1")
    banner = pg.locator(".banner[role=alert]")  # the Devices page has a Reload button of its own
    banner.wait_for(timeout=8000)
    check("ShellyLanMan was updated" in banner.inner_text() and banner.get_by_role("button", name="Reload").count() == 1,
          "another version: the banner asks to reload")
    check(pg.evaluate("window.__marker") == 1, "the page did not reload by itself")
    pg.goto(B + "#/settings"); pg.wait_for_timeout(800)
    check(pg.locator(".banner[role=alert]").count() == 1, "the banner stays on another page")
    pg.screenshot(path="/shots/newserver.png")
    pg.unroute_all()
    with pg.expect_navigation():
        pg.locator(".banner[role=alert]").get_by_role("button", name="Reload").click()
    check(pg.evaluate("typeof window.__marker") == "undefined", "Reload loads the page again")
    check(not [e for e in errors if "Content Security Policy" in e], "no CSP errors: %s" % errors)
    br.close()

if problems:
    raise SystemExit(1)
