"""Browser test of relayed BLU rows and the Identify BLU devices wizard.

Runs in the Playwright image against ShellyLanMan with simulated devices:

    SCRIPT=check-blu.py sh tools/screenshots/run.sh

The "Living room" simulator (Dimmer G3) relays an RC Button 4 and answers
BTHome.StartDeviceDiscovery with that button and an H&T that it does not
relay (tools/screenshots/blu). Exit 1 when:

- the relayed row is missing, not marked as an estimate, or offers backup,
  restore or logs;
- the wizard does not open from the row with the two-button hint, or does
  not show both answers, or the row does not get the identified model.

Screenshots: /shots/blu-row.png, blu-identify.png.
"""
import sys

from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"
problems = []


def check(ok, text):
    print(("ok   " if ok else "FAIL ") + text)
    if not ok:
        problems.append(text)


with sync_playwright() as p:
    br = p.chromium.launch()
    pg = br.new_page(viewport={"width": 1500, "height": 900})
    pg.add_init_script(QUIET)
    pg.goto(B + "#/devices")
    row = pg.locator("table.devices tbody tr", has_text="7C:C6:B6:A5:C9:3D")
    try:
        row.wait_for(timeout=30000)
        check(True, "relayed row present")
    except Exception:
        check(False, "relayed row present")
        pg.screenshot(path="/shots/blu-row.png")
        sys.exit(1)
    text = row.inner_text()
    check("?" in text, f"model is an estimate: {text!r}")
    row.locator("td.sel input").check()
    for name in ("Backup", "Restore", "Logs"):
        b = pg.get_by_role("button", name=name, exact=True)
        check(b.count() == 0 or b.first.is_disabled(), f"{name} disabled for a relayed row")
    pg.screenshot(path="/shots/blu-row.png")

    pg.get_by_role("button", name="Identify", exact=True).click()
    pg.wait_for_selector(".blu-identify", timeout=10000)
    body = pg.locator(".blu-identify")
    check("two of its buttons" in body.inner_text(), "two-button pairing hint for the RC Button 4")
    check("Living room" in body.locator("select").inner_text(), "gateway Living room offered")
    body.get_by_role("button", name="Start").click()
    pg.wait_for_selector(".blu-found tbody tr:nth-child(2)", timeout=15000)
    found = body.locator(".blu-found").inner_text()
    check("Button 4" in found and "SBHT-003C" in found, f"both answers shown: {found!r}")
    pg.locator(".blu-status", has_text="Done").wait_for(timeout=15000)
    pg.screenshot(path="/shots/blu-identify.png")
    pg.get_by_role("button", name="Close").last.click()
    pg.wait_for_timeout(2500)
    text = row.inner_text()
    check("RC Button 4" in text and "?" not in text, f"row identified: {text!r}")
    br.close()

if problems:
    print("FAILED:", *problems, sep="\n  ")
    sys.exit(1)
print("all ok")
