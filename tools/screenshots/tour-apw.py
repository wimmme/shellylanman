"""README animation of the access-point wizard: a recording of a running ShellyLanMan with
simulated devices, from the Firmware page to the device back with its new firmware.

    SCRIPT=tour-apw.py sh tools/screenshots/run.sh && NAME=tour-apw sh tools/screenshots/video.sh

The part on the phone (joining the access point, uploading the file at 192.168.33.1) happens
outside ShellyLanMan and is not in it. A simulator does not restart: the device's comeback
is played by answering the device list with a short uptime, and its firmware rows with the
new version, once the last step shows. Writes /shots/tour-apw/ (tourlib.py).
"""
import json

from playwright.sync_api import sync_playwright

from tourlib import B, Recorder, context

DEVICE = "Heat pump"  # its simulator says 2.0.3 is out (sims.sh)

with sync_playwright() as p:
    br = p.chromium.launch()
    pg = context(br).new_page()
    pg.goto(B + "#/firmware"); pg.wait_for_timeout(10000)  # Shelly's index, once
    # A phone address, as a real installation has (else the wizard warns that localhost does not work on a phone).
    pg.evaluate("fetch('api/v1/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phoneBaseURL: 'http://192.168.1.20:3082' }) })")
    dev = pg.evaluate("n => fetch('api/v1/devices').then(r => r.json()).then(l => l.find(d => d.name === n).id)", DEVICE)
    latest = pg.evaluate("id => fetch('api/v1/firmware/index?ids=' + id).then(r => r.json()).then(l => l[0].latest)", dev)

    back = {"on": False}

    def devices(route):
        resp = route.fetch()
        body = resp.json()
        if back["on"]:
            for d in body:
                if d.get("name") == DEVICE:
                    d["uptime"], d["updateAvailable"] = 3, False
        route.fulfill(response=resp, body=json.dumps(body))

    def firmware(route):  # the rows and Shelly's index: the device runs the new version
        resp = route.fetch()
        body = resp.json()
        if back["on"] and isinstance(body, list):
            for r in body:
                if r.get("id") == dev:
                    r["current"], r["currentBuild"], r["newer"], r["stable"] = latest, latest, False, ""
        route.fulfill(response=resp, body=json.dumps(body))
    def events(ws):  # the live events too: the device's own update mark is gone after the update
        server = ws.connect_to_server()
        ws.on_message(lambda m: server.send(m))

        def down(m):
            if back["on"] and isinstance(m, str) and '"device.upsert"' in m:
                ev = json.loads(m)
                if ev.get("data", {}).get("id") == dev:
                    ev["data"]["uptime"], ev["data"]["updateAvailable"] = 3, False
                    m = json.dumps(ev)
            ws.send(m)
        server.on_message(down)
    pg.route_web_socket("**/ws", events)
    pg.route("**/api/v1/devices", devices)
    pg.route("**/api/v1/firmware?*", firmware)
    pg.route("**/api/v1/firmware/index*", firmware)

    pg.goto(B + "#/devices"); pg.reload(); pg.wait_for_timeout(2000)
    pg.goto(B + "#/firmware"); pg.wait_for_timeout(6000)
    rec = Recorder(pg, "tour-apw")
    rec.hold(2.5)

    # The device's row: "through its access point".
    rec.click(pg.locator("table.fw tbody tr", has_text=DEVICE).locator("td").last.locator("button"))
    dlg = pg.locator(".modal[role=dialog]")
    rec.wait(2000)
    rec.hold(4)  # 1: the device, and a code that opens the wizard on the phone
    nxt = dlg.get_by_role("button", name="Next")
    for seconds in (5, 4.5, 4.5):  # 2: the firmware file; 3: join the access point; 4: open its page
        rec.click(nxt)
        rec.wait(3000)
        rec.hold(seconds / 2)
        # A step taller than the window: scroll to its end.
        dlg.evaluate("d => [d, ...d.querySelectorAll('*')].filter(e => e.scrollHeight > e.clientHeight + 20)"
                     ".forEach(e => e.scrollTo({ top: e.scrollHeight, behavior: 'smooth' }))")
        rec.hold(seconds / 2)

    # 5: waiting while the phone does its part; then the device is back with its new firmware.
    rec.click(nxt)
    rec.wait(1500)
    rec.hold(3)
    back["on"] = True
    dlg.locator("p[role=status]", has_text="is back").wait_for(timeout=20000)
    rec.wait(1500)
    rec.hold(3.5)
    rec.click(dlg.get_by_role("button", name="Close"))
    rec.hold(1)
    # The Firmware page, read again: the new version, no ⚡.
    pg.reload()
    rec.wait(6000)
    rec.hold(3.5)
    rec.save()
    br.close()
