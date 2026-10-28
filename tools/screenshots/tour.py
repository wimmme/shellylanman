"""The README tour: a recording of a running ShellyLanMan with simulated devices.

    SCRIPT=tour.py sh tools/screenshots/run.sh && sh tools/screenshots/video.sh

Devices (scroll, a device's menu with Device info and its Settings with every tab, one
device and then three selected), Checklist, Charts, Firmware with the access-point
wizard, Log, Settings with every tab, About. Writes /shots/tour/ (tourlib.py);
video.sh turns it into tour.mp4 and tour.gif.
"""
from playwright.sync_api import sync_playwright

from tourlib import B, H, Recorder, context


def row(pg, name):
    return pg.locator("table.devices tbody tr", has_text=name)


def pick(rec, name, ctrl=False):
    """A click on the device's name cell: selects it (with Ctrl: adds it)."""
    cell = row(rec.pg, name).locator("td", has_text=name).first
    if ctrl:
        rec.pg.keyboard.down("Control")
    rec.click(cell)
    if ctrl:
        rec.pg.keyboard.up("Control")


def menu(rec, name, item):
    rec.click(row(rec.pg, name).locator("td", has_text=name).first, button="right")
    rec.hold(0.8)
    rec.click(rec.pg.locator(".ctx-menu button", has_text=item))


def close(rec):
    rec.click(rec.pg.locator(".modal footer button").last)
    rec.hold(0.4)


with sync_playwright() as p:
    br = p.chromium.launch()
    pg = context(br).new_page()

    # Warm up: the Firmware page fetches Shelly's index once, the charts need history.
    pg.goto(B + "#/firmware"); pg.wait_for_timeout(8000)
    # A phone address, as a real installation has (else the wizard warns that localhost does not work on a phone).
    pg.evaluate("fetch('api/v1/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phoneBaseURL: 'http://192.168.1.20:3082' }) })")
    pg.wait_for_timeout(40000)
    pg.goto(B + "#/devices"); pg.reload(); pg.wait_for_timeout(3000)
    rec = Recorder(pg, "tour")

    # Devices: the whole list, scrolled down and back.
    rec.hold(2.5)
    rec.scroll(".main", 400, 1.5)
    rec.scroll(".main", 0, 1.2)

    # One device: its menu, Device info, then its Settings with every tab.
    pick(rec, "Garden pump")
    rec.hold(1.5)
    menu(rec, "Garden pump", "Device info")
    rec.wait(2500)
    rec.hold(3)
    close(rec)
    menu(rec, "Garden pump", "Settings")
    rec.wait(2500)
    rec.hold(1.5)
    tabs = pg.locator(".modal .cfg .tabs button")
    for i in range(1, tabs.count()):
        rec.click(tabs.nth(i))
        rec.wait(1200)
        rec.hold(1.3)
    close(rec)

    # One device selected, then three: the toolbar offers what fits the selection.
    pick(rec, "Porch light")
    rec.hold(1.5)
    pick(rec, "Living room", ctrl=True)
    pick(rec, "Heat pump", ctrl=True)
    rec.hold(2)

    # The same three on Checklist and Charts.
    rec.nav("Checklist")
    rec.wait(2500)
    rec.hold(3.5)
    rec.nav("Charts")
    rec.wait(2500)
    rec.hold(6)

    # Firmware, and the wizard that updates a device through its own access point.
    rec.nav("Firmware")
    rec.wait(4000)
    rec.hold(2.5)
    rec.click(pg.locator("table.fw tbody tr", has_text="Heat pump").locator("td").last.locator("button"))
    dlg = pg.locator(".modal[role=dialog]")
    rec.wait(2000)
    rec.hold(3)
    nxt = dlg.get_by_role("button", name="Next")
    for _ in range(3):  # firmware file, join the access point, open its page
        rec.click(nxt)
        rec.wait(2500)
        rec.hold(2.5)
    rec.click(dlg.get_by_role("button", name="Close"))
    rec.hold(0.5)

    # Log, Settings with every tab, About.
    rec.nav("Log")
    rec.wait(1500)
    rec.hold(3)
    rec.nav("Settings")
    rec.wait(1500)
    rec.hold(1.5)
    tabs = pg.locator(".tabs button[data-tab]")
    for i in range(1, tabs.count()):
        rec.click(tabs.nth(i))
        rec.wait(800)
        rec.hold(1.3)
    rec.nav("About")
    rec.wait(1500)
    rec.hold(4)
    rec.save()
    br.close()
