"""README screenshots of a running ShellyLanMan with simulated devices (see run.sh).

Runs in the Playwright image; writes PNGs to /shots. One look for all of them:
the Home Assistant palette (the start look) in dark mode.
"""
from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
OUT = "/shots/"
# The "no authentication" banner is for real installations, not for pictures.
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"


def page(browser, width=1440, height=900, scale=1):
    ctx = browser.new_context(color_scheme="dark", viewport={"width": width, "height": height}, device_scale_factor=scale)
    ctx.add_init_script(QUIET)
    return ctx.new_page()


def row(pg, name):
    return pg.locator("table.devices tbody tr", has_text=name)


def tick(pg, *names):
    for n in names:
        row(pg, n).locator("td.sel input").check()
    pg.wait_for_timeout(300)


def close_modal(pg):
    pg.locator(".modal footer button").last.click()
    pg.wait_for_timeout(300)


with sync_playwright() as p:
    br = p.chromium.launch()

    # Devices: a few ticked, wide enough for the Command column (the dimmers'
    # sliders) and the relayed BLU row, with the host name, cloud, MQTT, uptime and source columns hidden.
    pg = page(br, width=1920, height=1480)
    pg.context.add_init_script("""try { localStorage.setItem('sl_cols_default',
      JSON.stringify(['keyword', 'mac', 'ssid', 'logs', 'device', 'mqtt', 'uptime', 'source', 'cloud'])) } catch (e) {}""")
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    tick(pg, "Porch light", "Living room", "Heat pump")
    pg.evaluate("document.querySelector('.main').scrollTo(0, 0)"); pg.wait_for_timeout(300)
    pg.screenshot(path=OUT + "screenshot-devices.png")

    # The same selection on Checklist; then Firmware for all.
    pg = page(br)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    tick(pg, "Garden pump", "Hall buttons", "Porch light", "Kitchen strip", "Living room")
    pg.goto(B + "#/checklist"); pg.wait_for_timeout(5000)
    pg.screenshot(path=OUT + "screenshot-checklist.png")
    pg.evaluate("sessionStorage.removeItem('sl_selection')")
    pg.goto(B + "#/firmware"); pg.reload(); pg.wait_for_timeout(8000)
    pg.screenshot(path=OUT + "screenshot-firmware.png")

    # Device info and the live log of one device.
    pg = page(br)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    tick(pg, "Car charger")
    pg.get_by_role("button", name="Device info").click(); pg.wait_for_timeout(2500)
    pg.screenshot(path=OUT + "screenshot-info.png")
    close_modal(pg)
    row(pg, "Car charger").locator("td.sel input").uncheck()
    tick(pg, "Porch light")
    pg.get_by_role("button", name="Logs").click(); pg.wait_for_timeout(4500)
    pg.screenshot(path=OUT + "screenshot-logs.png")
    close_modal(pg)

    # The script editor (the "Heat pump" simulator has a script, tools/screenshots/script).
    row(pg, "Porch light").locator("td.sel input").uncheck()
    tick(pg, "Heat pump")
    pg.get_by_role("button", name="Scripts").click()
    pg.wait_for_selector("input.scr-name", timeout=15000)
    pg.locator('tr:has(input.scr-name[value="power-limit"])').dispatch_event("dblclick")
    pg.wait_for_selector(".ide-editor .cm-line", timeout=15000); pg.wait_for_timeout(800)
    pg.screenshot(path=OUT + "screenshot-editor.png")

    # Charts of three devices, after some readings came in.
    pg = page(br)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    tick(pg, "Garden pump", "Porch light", "Living room")  # all three report a temperature
    pg.goto(B + "#/charts"); pg.wait_for_timeout(45000)
    pg.screenshot(path=OUT + "screenshot-charts.png")

    pg = page(br)
    pg.goto(B + "#/about"); pg.wait_for_timeout(2000)
    pg.screenshot(path=OUT + "screenshot-about.png")

    # Identify a BLU device that "Living room" relays (tools/screenshots/blu).
    pg = page(br)
    pg.goto(B + "#/devices"); pg.wait_for_timeout(3000)
    tick(pg, "7c:c6:b6:a5:c9:3d")
    pg.get_by_role("button", name="Identify", exact=True).click()
    pg.wait_for_selector(".blu-identify")
    pg.locator(".blu-identify").get_by_role("button", name="Start").click()
    pg.wait_for_selector(".blu-found tbody tr:nth-child(2)", timeout=15000)
    pg.locator(".blu-status", has_text="Done").wait_for(timeout=15000)
    pg.screenshot(path=OUT + "screenshot-identify.png")

    # A phone: Checklist, and the menu.
    ph = page(br, 390, 844, 2)
    ph.goto(B + "#/checklist"); ph.wait_for_timeout(4000)
    ph.screenshot(path=OUT + "screenshot-phone.png")
    ph.click(".menu-btn"); ph.wait_for_timeout(500)
    ph.screenshot(path=OUT + "screenshot-phone-menu.png")
    br.close()
