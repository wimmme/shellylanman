"""Browser test of the tables on a phone and on a wide desktop: Devices, Checklist and Firmware.

    SCRIPT=check-phone-tables.py sh tools/screenshots/run.sh

On a phone (390 px) each device must be a card: nothing to scroll sideways, no column headings,
the name and the status on the card's first line, a column name before each value, and on Firmware
the "Via access point" button (when Shelly's index has a newer version) on the screen. On a
1920 px desktop the tables stay tables; Checklist fits without scrolling sideways with the host
name under the name, and the Devices table is never taller than the screen, so its sideways
scroll bar is in view. Exit 1 when one of these fails.
Screenshots: /shots/phone-devices.png, /shots/phone-checklist.png, /shots/phone-firmware.png,
/shots/desk-devices.png, /shots/desk-checklist.png.
"""
import sys
import time

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


PAGES = {  # page: (table, name cell, status cell)
    "devices": ("table.devices", "td.c-name", "td.c-status"),
    "checklist": ("table.checklist", "td.chk-host", "td.chk-status"),
    "firmware": ("table.fw", "td.fw-name", "td.fw-status"),
}


def open_page(pg, page):
    table = PAGES[page][0]
    pg.goto(B + "#/" + page)
    pg.wait_for_selector(table + " tbody tr", timeout=30000)
    # Rows are read six at a time (and Firmware asks Shelly's index): wait until all are in.
    until(lambda: pg.locator(table + " tbody .spinner").count() == 0
          and pg.locator(table + " td[data-label]", has_text="…").count() == 0)
    pg.wait_for_timeout(500)


def context(br, **kw):
    ctx = br.new_context(color_scheme="dark", **kw)
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1'); localStorage.setItem('sl_view', 'detailed') } catch (e) {}")
    return ctx


with sync_playwright() as p:
    br = p.chromium.launch()
    errors = []

    phone = context(br, viewport={"width": 390, "height": 844}, device_scale_factor=2, is_mobile=True, has_touch=True).new_page()
    phone.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    for page, (table, name, status) in PAGES.items():
        open_page(phone, page)
        sw, cw = phone.locator(table).locator("xpath=..").evaluate("e => [e.scrollWidth, e.clientWidth]")
        check(sw <= cw, f"{page}: no sideways scrolling ({sw} <= {cw})")
        check(phone.evaluate("() => document.documentElement.scrollWidth") <= 390, f"{page}: the page is not wider than the phone")
        check(not phone.locator(table + " thead").is_visible(), f"{page}: no column headings")
        first = phone.locator(table + " tbody tr").first
        nb, sb = first.locator(name).bounding_box(), first.locator(status).bounding_box()
        check(nb is not None and sb is not None and sb["x"] > nb["x"] and sb["y"] < nb["y"] + nb["height"],
              f"{page}: name and status on the card's first line")
        labels = first.locator("td[data-label]").evaluate_all(
            "tds => tds.filter(td => getComputedStyle(td).display !== 'none').map(td => getComputedStyle(td, '::before').content)")
        check(len(labels) >= 3 and all(l not in ("none", "normal", '""') for l in labels), f"{page}: a column name before each value {labels}")
        if page == "firmware":
            btn = phone.locator(table + " button", has_text="⚡")
            if btn.count():
                b = btn.first.bounding_box()
                check(b["x"] >= 0 and b["x"] + b["width"] <= 390, f"firmware: 'Via access point' on the screen (x {b['x']:.0f}..{b['x'] + b['width']:.0f})")
            else:
                print("note no newer version in Shelly's index (offline?): button not checked")
        if page == "checklist":
            ys = [round(b["y"]) for b in first.locator("td.chk").evaluate_all("tds => tds.map(td => td.getBoundingClientRect().toJSON())")]
            check(len(set(ys)) * 2 >= len(ys) and len(set(ys)) < len(ys), f"checklist: two settings per line ({len(ys)} on {len(set(ys))} lines)")
        # .main scrolls, not the page: show the first cards rather than a full-page shot.
        first.evaluate("e => e.scrollIntoView()")
        phone.screenshot(path=f"/shots/phone-{page}.png")

    desk = context(br, viewport={"width": 1920, "height": 1080}).new_page()
    desk.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    open_page(desk, "checklist")
    check(desk.locator("table.checklist thead").is_visible(), "desktop checklist: column headings")
    sw, cw = desk.locator(".table-wrap").first.evaluate("e => [e.scrollWidth, e.clientWidth]")
    check(sw <= cw, f"desktop checklist: no sideways scrolling ({sw} <= {cw})")
    two = desk.locator("table.checklist td.chk-host").evaluate_all("tds => tds.filter(td => td.children.length === 2).length")
    check(two > 0, f"desktop checklist: the host name under the name ({two} rows)")
    desk.screenshot(path="/shots/desk-checklist.png")
    open_page(desk, "devices")
    check(desk.locator("table.devices tbody tr").first.evaluate("e => getComputedStyle(e).display") == "table-row", "desktop devices: rows are table rows")
    wrap = desk.locator(".table-wrap.fit").first
    wrap.scroll_into_view_if_needed()
    desk.locator(".main").evaluate("m => m.scrollTo(0, m.scrollHeight)")
    wb = wrap.bounding_box()
    check(wb["y"] + wb["height"] <= 1080 + 1, f"desktop devices: the table ends on the screen (bottom {wb['y'] + wb['height']:.0f})")
    sw, cw = wrap.evaluate("e => [e.scrollWidth, e.clientWidth]")
    print(f"note desktop devices, detailed view: {sw} px of table in {cw} px")
    th0 = desk.locator("table.devices thead th").nth(2).bounding_box()["y"]
    wrap.evaluate("e => e.scrollTo(0, e.scrollHeight)")
    th1 = desk.locator("table.devices thead th").nth(2).bounding_box()["y"]
    check(abs(th1 - th0) < 2, f"desktop devices: the headings stay put while the table scrolls ({th0:.0f} -> {th1:.0f})")
    desk.screenshot(path="/shots/desk-devices.png")
    check(not errors, f"no console errors {errors}")
    br.close()

print("FAILED: " + "; ".join(problems) if problems else "all ok")
sys.exit(1 if problems else 0)
