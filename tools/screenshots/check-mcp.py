"""Browser test of Settings → MCP in a fresh installation (DECISIONS P23-1).

    SCRIPT=check-mcp.py sh tools/screenshots/run.sh

Exit 1 when MCP is not on, the access level is not "read only", or the page does
not say that a token still has to be made. Screenshot: /shots/mcp.png.
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
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.goto(B + "#/settings"); pg.wait_for_selector(".server-ports", timeout=10000)
    pg.locator("[data-tab=mcp]").click()
    pg.wait_for_selector("#mcpOn", timeout=10000)
    check(pg.locator("#mcpOn").is_checked(), "MCP on by default")
    check(pg.locator("#mcpAccess").input_value() == "read", "read only by default")
    check(pg.get_by_text("No token yet").count() == 1, "says a token still has to be made")
    check(pg.get_by_role("button", name="New token").is_enabled(), "New token can be pressed")
    check(not errors, f"no console errors (CSP): {errors}")
    pg.screenshot(path="/shots/mcp.png")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
