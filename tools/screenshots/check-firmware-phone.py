"""Browser test of the Firmware page on a phone: each device a card, nothing to scroll sideways.

    SCRIPT=check-firmware-phone.py sh tools/screenshots/run.sh

Exit 1 when, at phone width, the table needs scrolling sideways, the column headings still show,
a card does not have its name and status on its first line, a value has no column name, or the
"Via access point" button (when Shelly's index has a newer version) lies outside the screen; and
when the desktop table is no longer a plain table. Screenshots: /shots/fw-phone.png,
/shots/fw-desktop.png.
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


def firmware(pg):
    pg.goto(B + "#/firmware")
    pg.wait_for_selector("table.fw tbody tr", timeout=30000)
    # Rows are read six at a time and the index comes from the internet: wait until both are done.
    until(lambda: pg.locator("table.fw td.fw-status .spinner").count() == 0
          and pg.locator("table.fw td[data-label]", has_text="…").count() == 0)


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 390, "height": 844}, device_scale_factor=2,
                         is_mobile=True, has_touch=True)
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    firmware(pg)

    wrap = pg.locator(".fw-panel .table-wrap").first
    sw, cw = wrap.evaluate("e => [e.scrollWidth, e.clientWidth]")
    check(sw <= cw, f"no sideways scrolling ({sw} <= {cw})")
    check(not pg.locator("table.fw thead").is_visible(), "no column headings on a phone")
    rows = pg.locator("table.fw tbody tr")
    check(rows.count() > 0, f"{rows.count()} cards")
    first = rows.first
    name_box = first.locator("td.fw-name").bounding_box()
    status_box = first.locator("td.fw-status").bounding_box()
    check(abs((name_box["y"] + name_box["height"] / 2) - (status_box["y"] + status_box["height"] / 2)) < 12
          and status_box["x"] > name_box["x"], "name and status on the card's first line")
    labels = first.locator("td[data-label]").evaluate_all(
        "tds => tds.map(td => getComputedStyle(td, '::before').content)")
    check(len(labels) == 5 and all(l not in ("none", "normal", '""') for l in labels),
          f"every value has its column name {labels}")
    btn = pg.locator("table.fw button", has_text="⚡")
    if btn.count():
        b = btn.first.bounding_box()
        check(b["x"] >= 0 and b["x"] + b["width"] <= 390, f"'Via access point' on the screen (x {b['x']:.0f}..{b['x'] + b['width']:.0f})")
    else:
        print("note no newer version in Shelly's index (offline?): button not checked")
    page_w = pg.evaluate("() => document.documentElement.scrollWidth")
    check(page_w <= 390, f"the page itself is not wider than the phone ({page_w})")
    pg.screenshot(path="/shots/fw-phone.png", full_page=True)
    check(not errors, f"no console errors {errors}")

    # The desktop keeps its table.
    dk = br.new_context(color_scheme="dark", viewport={"width": 1440, "height": 900})
    dk.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    dp = dk.new_page()
    firmware(dp)
    check(dp.locator("table.fw thead").is_visible(), "column headings on the desktop")
    check(dp.locator("table.fw tbody tr").first.evaluate("e => getComputedStyle(e).display") == "table-row",
          "rows are table rows on the desktop")
    check(dp.locator("table.fw td[data-label]").first.evaluate("e => getComputedStyle(e, '::before').content") in ("none", "normal"),
          "no column names in the cells on the desktop")
    dp.screenshot(path="/shots/fw-desktop.png")
    br.close()

print("FAILED: " + "; ".join(problems) if problems else "all ok")
sys.exit(1 if problems else 0)
