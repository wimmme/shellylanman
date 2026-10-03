"""The script editor renders under the real Content-Security-Policy.

Runs in the Playwright image against ShellyLanMan with simulated devices:

    SCRIPT=check-editor.py sh tools/screenshots/run.sh

Opens the long demo script of "Heat pump" (tools/screenshots/script) in the
editor, light and dark (the editor follows the app's theme by default), and fails (exit 1) when CodeMirror's stylesheet was not
applied: the scroller must scroll (overflow not 'visible'), the first line must
be inside the editor box, and the browser must not have refused an inline style
(a CSP without the nonce did exactly that: an unstyled, garbled editor).
Screenshots: /shots/editor-light.png, /shots/editor-dark.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"
problems = []

with sync_playwright() as p:
    br = p.chromium.launch()
    for scheme in ("light", "dark"):
        ctx = br.new_context(color_scheme=scheme, viewport={"width": 1440, "height": 900})
        ctx.add_init_script(QUIET)  # editor colours: the default, following the app
        pg = ctx.new_page()
        refused = []
        pg.on("console", lambda m: refused.append(m.text) if "Refused to apply inline style" in m.text else None)
        pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
        pg.locator("table.devices tbody tr", has_text="Heat pump").locator("td.sel input").check()
        pg.get_by_role("button", name="Scripts").click()
        pg.wait_for_selector("input.scr-name", timeout=15000)  # the script list (names are inputs)
        pg.locator("tr:has(input.scr-name)").first.dispatch_event("dblclick")  # double-click opens the editor
        pg.wait_for_selector(".ide-editor .cm-line", timeout=15000)
        pg.wait_for_timeout(500)
        r = pg.evaluate("""() => {
          const scroller = document.querySelector('.ide-editor .cm-scroller');
          const box = document.querySelector('.ide-editor').getBoundingClientRect();
          const first = document.querySelector('.ide-editor .cm-line').getBoundingClientRect();
          return { overflow: getComputedStyle(scroller).overflow, first: first.top, top: box.top, bottom: box.bottom,
                   dark: document.querySelector('.ide-editor').classList.contains('dark'),
                   text: document.querySelector('.ide-editor .cm-line').textContent };
        }""")
        print(scheme, r, "refused:", len(refused))
        if r["overflow"] == "visible":
            problems.append(f"{scheme}: .cm-scroller overflow is visible (CodeMirror's stylesheet not applied)")
        if not (r["top"] <= r["first"] < r["bottom"]):
            problems.append(f"{scheme}: first line at y={r['first']}, editor box {r['top']}–{r['bottom']}")
        if r["dark"] != (scheme == "dark"):
            problems.append(f"{scheme}: the editor does not follow the app's theme (dark={r['dark']})")
        if refused:
            problems.append(f"{scheme}: CSP refused an inline style: {refused[0][:120]}")
        pg.screenshot(path=f"/shots/editor-{scheme}.png")
        ctx.close()
    br.close()

if problems:
    print("FAIL\n" + "\n".join(problems))
    sys.exit(1)
print("OK: the script editor is styled, light and dark")
