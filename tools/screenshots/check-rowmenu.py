"""Browser test of the right-click menu on Devices and Checklist (DECISIONS P31-1).

    SCRIPT=check-rowmenu.py sh tools/screenshots/run.sh

Exit 1 when a right-click on a row inside a selection of several devices keeps
that selection (the menu would act on all of them, Web UI opening every page),
or a right-click outside it does not select only that row.

Screenshot: /shots/rowmenu.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def selected(pg, table):
    return pg.eval_on_selector_all(table + " tbody tr.selected", "rs => rs.map(r => r.dataset.id)")


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1920, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)

    for page, table in (("devices", "table.devices"), ("checklist", "table.checklist")):
        pg.goto(B + "#/" + page)
        pg.wait_for_selector(table + " tbody tr", timeout=30000)
        pg.wait_for_timeout(1500)
        # A selection from the page before limits the Checklist to it: show all devices.
        if pg.locator(".banner.scope button").count():
            pg.locator(".banner.scope button").first.click(); pg.wait_for_timeout(1500)
        rows = pg.locator(table + " tbody tr")
        check(rows.count() >= 4, f"{page}: at least 4 rows ({rows.count()})")
        ids = [rows.nth(i).get_attribute("data-id") for i in range(4)]

        # Three rows selected, right-click on the second: only that row stays selected.
        rows.nth(0).locator("td").nth(2).click()
        rows.nth(1).locator("td").nth(2).click(modifiers=["Control"])
        rows.nth(2).locator("td").nth(2).click(modifiers=["Control"])
        check(len(selected(pg, table)) == 3, f"{page}: three rows selected {selected(pg, table)}")
        rows.nth(1).locator("td").nth(2).click(button="right")
        pg.wait_for_selector(".ctx-menu", timeout=5000)
        check(selected(pg, table) == [ids[1]], f"{page}: right-click inside the selection selects that row only {selected(pg, table)}")
        if page == "devices":
            pg.screenshot(path="/shots/rowmenu.png")
        pg.keyboard.press("Escape"); pg.mouse.click(5, 5); pg.wait_for_timeout(300)

        # Right-click outside a selection of two: that row only.
        rows.nth(0).locator("td").nth(2).click()
        rows.nth(1).locator("td").nth(2).click(modifiers=["Control"])
        rows.nth(3).locator("td").nth(2).click(button="right")
        pg.wait_for_selector(".ctx-menu", timeout=5000)
        check(selected(pg, table) == [ids[3]], f"{page}: right-click outside the selection selects that row only")
        pg.mouse.click(5, 5); pg.wait_for_timeout(300)

    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
