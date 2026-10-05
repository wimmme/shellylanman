"""Browser test of the full / minimal sidebar (as in Home Assistant).

    SCRIPT=check-nav.py sh tools/screenshots/run.sh

Exit 1 when the toggle is missing on a wide screen, the minimal sidebar is not
narrow with icons only and tooltips, the choice is not remembered after a
reload, or the toggle shows on a phone (there the ☰ drawer stays).

Screenshots: /shots/nav-full.png, nav-mini.png, nav-phone.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def width(pg, sel):
    return pg.eval_on_selector(sel, "e => e.getBoundingClientRect().width")


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    pg.goto(B + "#/devices"); pg.wait_for_timeout(2500)

    btn = pg.locator(".side-btn")
    check(btn.is_visible() and btn.get_attribute("title") == "Collapse sidebar", "toggle shown, full by default")
    check(width(pg, ".sidebar") > 150 and pg.locator(".nav .label").first.is_visible(), "full: labels visible")
    brand = pg.eval_on_selector(".brand", "b => [...b.children].map(c => c.tagName)")
    check(brand == ["BUTTON", "SPAN", "IMG"], f"brand: toggle, name, logo right of the name {brand}")
    pg.screenshot(path="/shots/nav-full.png")

    btn.click(); pg.wait_for_timeout(500)
    w = width(pg, ".sidebar")
    check(w < 60, f"minimal: sidebar {w}px")
    check(not pg.locator(".nav .label").first.is_visible() and not pg.locator(".brand img").is_visible(), "minimal: no labels, no logo")
    check(pg.locator(".nav a").first.get_attribute("title") == "Devices", "minimal: tooltips on the icons")
    check(pg.locator(".side-btn").get_attribute("title") == "Expand sidebar", "toggle now expands")
    pg.screenshot(path="/shots/nav-mini.png")

    pg.reload(); pg.wait_for_timeout(2500)
    check(width(pg, ".sidebar") < 60, "minimal remembered after a reload")
    pg.locator(".side-btn").click(); pg.wait_for_timeout(500)
    check(width(pg, ".sidebar") > 150, "back to full")

    # A phone: the drawer behind ☰, no sidebar toggle, even when minimal was chosen.
    pg.locator(".side-btn").click()
    ph = ctx.new_page()
    ph.set_viewport_size({"width": 390, "height": 844})
    ph.goto(B + "#/devices"); ph.wait_for_timeout(2500)
    check(ph.locator(".menu-btn").is_visible(), "phone: ☰ shown")
    ph.click(".menu-btn"); ph.wait_for_timeout(500)
    check(not ph.locator(".side-btn").is_visible() and ph.locator(".nav .label").first.is_visible(), "phone drawer: labels, no toggle")
    ph.screenshot(path="/shots/nav-phone.png")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
