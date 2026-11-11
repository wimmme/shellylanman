"""Browser test: the download progress of a firmware update keeps moving after
leaving the Firmware page and coming back.

    SCRIPT=check-fwprogress.py sh tools/screenshots/run.sh

The simulated Mini PM Gen3 ("Heat pump", sims.sh) reports ota_progress 0..100 %
over 20 s after Shelly.Update. Exit 1 when the percentage stops moving.
"""
import re
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def percent(pg):
    row = pg.locator("table.fw tbody tr", has_text="Heat pump")
    m = re.search(r"load (\d+)%", row.inner_text())
    return int(m.group(1)) if m else None


with sync_playwright() as p:
    br = p.chromium.launch()
    ctx = br.new_context(color_scheme="dark", viewport={"width": 1920, "height": 1000})
    ctx.add_init_script("try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}")
    pg = ctx.new_page()
    errors = []
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)

    pg.goto(B + "#/firmware")
    row = pg.locator("table.fw tbody tr", has_text="Heat pump")
    row.wait_for(timeout=30000)
    pg.wait_for_function("() => [...document.querySelectorAll('table.fw tbody tr')].some(r => r.textContent.includes('Heat pump') && r.querySelector('input[type=checkbox]:not([disabled])'))", timeout=30000)
    pg.locator("button", has_text="Deselect all").click()
    row.locator("label.fw-check input").first.check()
    pg.get_by_role("button", name="Update", exact=True).click()
    pg.locator(".modal button", has_text="Update").click()
    pg.wait_for_function("() => /load \d+%/.test([...document.querySelectorAll('table.fw tbody tr')].find(r => r.textContent.includes('Heat pump'))?.textContent ?? '')", timeout=20000)
    pg.wait_for_timeout(3000)
    a = percent(pg)
    check(a is not None, f"progress shown before leaving ({a}%)")

    pg.goto(B + "#/devices"); pg.wait_for_selector("table.devices tbody tr", timeout=20000); pg.wait_for_timeout(1500)
    pg.goto(B + "#/firmware")
    pg.locator("table.fw tbody tr", has_text="Heat pump").wait_for(timeout=20000)
    pg.wait_for_timeout(2000)
    b = percent(pg)
    pg.wait_for_timeout(4000)
    c = percent(pg)
    pg.screenshot(path="/shots/fw-progress.png")
    check(b is not None and c is not None and c > b, f"progress moves after coming back ({a}% -> {b}% -> {c}%)")
    check(not errors, f"no console errors (CSP): {errors}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
