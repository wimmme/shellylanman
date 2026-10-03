"""README screenshots of a running ShellyLanMan with simulated devices (see run.sh).

Runs in the Playwright Python image; writes PNGs to /shots. The start look is the
Home Assistant palette, light or dark as the (headless) system prefers, so the
colour scheme of each context chooses the mode.
"""
from playwright.sync_api import sync_playwright

B = "http://127.0.0.1:3082/"
OUT = "/shots/"
DESKTOP = {"width": 1440, "height": 900}
PHONE = {"width": 390, "height": 844}
# The "no authentication" banner is for real installations, not for pictures.
QUIET = "try { localStorage.setItem('sl_noauth_dismissed', '1') } catch (e) {}"


def context(browser, scheme, viewport, scale=1):
    ctx = browser.new_context(color_scheme=scheme, viewport=viewport, device_scale_factor=scale)
    ctx.add_init_script(QUIET)
    return ctx


def tick(page, rows):
    boxes = page.locator("table.devices tbody td.sel input")
    for i in rows:
        boxes.nth(i).check()


with sync_playwright() as p:
    br = p.chromium.launch()

    dark = context(br, "dark", DESKTOP).new_page()
    dark.goto(B + "#/devices"); dark.wait_for_timeout(3000)
    tick(dark, (1, 4, 6)); dark.wait_for_timeout(500)
    dark.screenshot(path=OUT + "screenshot-devices.png")
    dark.goto(B + "#/about"); dark.wait_for_timeout(2000)
    dark.screenshot(path=OUT + "screenshot-about.png")

    light = context(br, "light", DESKTOP).new_page()
    light.goto(B + "#/devices"); light.wait_for_timeout(3000)
    tick(light, (0, 2, 4, 5, 8)); light.wait_for_timeout(300)
    light.goto(B + "#/checklist"); light.wait_for_timeout(5000)
    light.screenshot(path=OUT + "screenshot-checklist.png")
    light.evaluate("sessionStorage.removeItem('sl_selection')")  # Firmware for all devices
    light.goto(B + "#/firmware"); light.reload(); light.wait_for_timeout(8000)
    light.screenshot(path=OUT + "screenshot-firmware.png")

    phone = context(br, "dark", PHONE, 2).new_page()
    phone.goto(B + "#/checklist"); phone.wait_for_timeout(4000)
    phone.screenshot(path=OUT + "screenshot-phone.png")
    phone.click(".menu-btn"); phone.wait_for_timeout(500)
    phone.screenshot(path=OUT + "screenshot-phone-menu.png")
    br.close()
