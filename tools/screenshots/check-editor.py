"""Browser test of the script editor under the real Content-Security-Policy.

Runs in the Playwright image against ShellyLanMan with simulated devices:

    SCRIPT=check-editor.py sh tools/screenshots/run.sh

The "Heat pump" simulator has three scripts (tools/screenshots/script):
power-limit (long code), slow-device (Script.GetCode answers after 4 s) and
broken-read (Script.GetCode fails with HTTP 500). Exit 1 when:

- the editor is unstyled (CodeMirror's stylesheet blocked by the CSP: the
  scroller does not scroll, the first line is outside the editor box, or the
  browser refused an inline style), or does not follow the app's light/dark
  theme;
- a slow device gives no feedback at once, or a second double-click opens a
  second editor, or the uploads are enabled before the code is there;
- a failing read shows an editable (empty) editor or enables the uploads
  instead of an error with Retry.

Screenshots: /shots/editor-light.png, editor-dark.png, editor-loading.png,
editor-failed.png.
"""
import sys
import time

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


def open_scripts(pg):
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    pg.locator("table.devices tbody tr", has_text="Heat pump").locator("td.sel input").check()
    pg.get_by_role("button", name="Scripts").click()
    pg.wait_for_selector("input.scr-name", timeout=15000)  # the script list (names are inputs)


def row(pg, name):
    return pg.locator(f'tr:has(input.scr-name[value="{name}"])')


def close_editor(pg):
    pg.locator(".modal.full footer button").click()
    pg.wait_for_timeout(300)


def uploads_disabled(pg):
    return pg.evaluate("""() => [...document.querySelectorAll('.ide-toolbar button')]
      .filter((b) => /Upload/.test(b.textContent)).every((b) => b.disabled)""")


with sync_playwright() as p:
    br = p.chromium.launch()

    # 1. Styled under the CSP, following the app's theme (light and dark).
    for scheme in ("light", "dark"):
        ctx = br.new_context(color_scheme=scheme, viewport={"width": 1440, "height": 900})
        ctx.add_init_script(QUIET)  # editor colours: the default, following the app
        pg = ctx.new_page()
        refused = []
        pg.on("console", lambda m: refused.append(m.text) if "Refused to apply inline style" in m.text else None)
        open_scripts(pg)
        row(pg, "power-limit").dispatch_event("dblclick")  # double-click opens the editor
        pg.wait_for_selector(".ide-editor .cm-line", timeout=15000)
        pg.wait_for_timeout(500)
        r = pg.evaluate("""() => {
          const scroller = document.querySelector('.ide-editor .cm-scroller');
          const box = document.querySelector('.ide-editor').getBoundingClientRect();
          const first = document.querySelector('.ide-editor .cm-line').getBoundingClientRect();
          return { overflow: getComputedStyle(scroller).overflow, first: first.top, top: box.top, bottom: box.bottom,
                   dark: document.querySelector('.ide-editor').classList.contains('dark') };
        }""")
        check(r["overflow"] != "visible", f"{scheme}: the editor scrolls (overflow {r['overflow']})")
        check(r["top"] <= r["first"] < r["bottom"], f"{scheme}: first line inside the editor (y={r['first']:.0f}, box {r['top']:.0f}-{r['bottom']:.0f})")
        check(not refused, f"{scheme}: no inline style refused by the CSP" + (f" ({refused[0][:80]})" if refused else ""))
        check(r["dark"] == (scheme == "dark"), f"{scheme}: the editor follows the app's theme")
        pg.screenshot(path=f"/shots/editor-{scheme}.png")
        ctx.close()

    ctx = br.new_context(color_scheme="light", viewport={"width": 1440, "height": 900})
    ctx.add_init_script(QUIET)
    pg = ctx.new_page()
    open_scripts(pg)

    # 2. A slow device: feedback at once, one editor for two double-clicks, uploads off until the code is there.
    t0 = time.monotonic()
    row(pg, "slow-device").dispatch_event("dblclick")
    row(pg, "slow-device").dispatch_event("dblclick")
    pg.wait_for_selector(".ide-state[role=status]", timeout=2000)
    check(time.monotonic() - t0 < 1.0, f"slow: loading shown after {time.monotonic() - t0:.2f} s")
    check(pg.locator(".modal.full").count() == 1, f"slow: one editor window for two double-clicks ({pg.locator('.modal.full').count()})")
    check(uploads_disabled(pg), "slow: Upload / Upload and run disabled while loading")
    check(pg.locator(".ide-editor .cm-editor").count() == 0, "slow: no editor before the code is there")
    pg.screenshot(path="/shots/editor-loading.png")
    pg.wait_for_selector(".ide-editor .cm-line", timeout=15000)
    check(time.monotonic() - t0 > 3.5, f"slow: the code arrived after {time.monotonic() - t0:.1f} s (the device's delay)")
    check(not uploads_disabled(pg), "slow: uploads enabled once the code is there")
    close_editor(pg)

    # 3. A failing read: an error with Retry, no editor, uploads off.
    row(pg, "broken-read").dispatch_event("dblclick")
    pg.wait_for_selector(".ide-state.error", timeout=15000)
    check(pg.locator(".ide-editor .cm-editor").count() == 0, "failing: no (empty) editor")
    check(uploads_disabled(pg), "failing: Upload / Upload and run stay disabled")
    check(pg.get_by_role("button", name="Retry").count() == 1, "failing: Retry offered")
    pg.screenshot(path="/shots/editor-failed.png")
    pg.get_by_role("button", name="Retry").click()
    pg.wait_for_selector(".ide-state.error", timeout=15000)
    check(pg.locator(".ide-editor .cm-editor").count() == 0, "failing: still no editor after Retry")
    close_editor(pg)
    br.close()

if problems:
    print(f"FAIL: {len(problems)} problem(s)")
    sys.exit(1)
print("OK: script editor styled, follows the theme, shows loading and errors, never opens blind")
