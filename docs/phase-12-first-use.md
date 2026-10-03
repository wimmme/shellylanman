# Phase 12 — First use: one selection, clearer pages (analysis, 2026-10-03)

Wim used ShellyLanMan 0.5.0 for the first time as a user, looking at how the app
itself works rather than at feature parity. Layout and technology are fine; what is
missing is a logic a first-time user can follow without knowing ShellyScanner.

This phase **knowingly leaves ShellyScanner's behaviour** where that behaviour came
from a fat Java client that grew organically. ShellyScanner stays the functional
reference for *what* a feature does and which Shelly API it uses; *how the pages work
together* is now ShellyLanMan's own. The About page and GitHub say so: started from
ShellyScanner, built on its basis, developed further (§4, item 2 of
`docs/phase-11-ha-mcp.md` §7).

Nothing in this document is built yet. §5 holds the questions, §6 the plan.

## 1. Wim's feedback (2026-10-03, as written, grouped)

**Devices and Checklist feel like two pages for the same thing.** Both seem to run
tasks on many devices.

1. Devices selected on *Devices* should be ticked on *Checklist* when Checklist is
   opened from the sidebar — as the *Checklist* button on Devices already does.
2. On *Devices*, *Checklist* belongs right after *Device selection*, before *Device info*.
3. On *Devices*, group the buttons: everything that works on several devices together;
   what works on one device only (info, logs, scheduler, scripts, notes) at the end.
4. *Reload*, *Refresh*, *Rescan*: unclear. Refresh and Rescan have a tooltip that should
   say plainly: Refresh reads the status of the devices in the list, Rescan rebuilds the
   list. Reload has no tooltip and its purpose is unknown — maybe superfluous.
5. *Checklist* has no checkboxes; it should, filled from *Devices* or the other way round.
6. Because Checklist filters on the ticked devices, it needs *Show all devices* (clear
   filter) next to *Refresh*.
7. The Checklist action buttons (Eco, LED, Logs, Bluetooth, AP, Roaming, Extender,
   Web UI, Reboot) **never become active**; Wim does not know how to use them.
8. *Web UI* for several devices: always ask first, saying it opens one tab per device.
9. *Firmware* works differently again. Expected: nothing selected → all devices;
   devices selected on Devices or Checklist → only those, with *Show all*.
10. *Firmware* shows a spinner until every device has answered, then the whole table;
    Checklist shows the rows at once and fills them in. Make Firmware behave like Checklist.

**Home Assistant**

11. Do the Shellys have to be added with the official Shelly integration first, or was
    the idea that ShellyLanMan adds all found Shellys to Home Assistant at once?
12. In Home Assistant they show as linked devices, not as one device.
13. Log warning: `custom integration 'shellylanman' calls device_registry.async_get_or_create
    with a deprecated default_manufacturer parameter; use manufacturer instead
    (binary_sensor.py line 35) … will stop working in Home Assistant 2027.9.0`.
14. "ShellyLanMan app → dit zou wel handig zijn", with the app log lines
    `Successfully send discovery information to Home Assistant` — meaning to be confirmed (Q6).
15. Must be installable as a Home Assistant app *or* as a standalone Docker container.

**Look and devices**

16. Rename the palettes *Default* dark / light; make *Home Assistant* (dark and light)
    the default everywhere, not only inside Home Assistant.
17. Phone: the navigation is at the bottom in portrait and at the left in landscape.
    Better vertical, and fixed.
18. Is the site installable as a PWA? No — can it be?

**Added later the same day**

19. After *Rescan*, almost every device shows *archived* at once. An in-between status
    (*searching*) first, and *archived* only later?
20. ShellyTestPlug stood in *error* while it was certainly on line. Unclear how to fix
    it: select and *Reload*? *Refresh*? *Rescan* fixed it, but rescans every device —
    with one device selected, should *Rescan* be dimmed?
21. Write *online* instead of *on line*.
22. *Web UI* opens a new tab: show the usual ↗ arrow, and make new tab / same tab a
    setting. Fully Kiosk Browser (HA frontend on a wall tablet) does not allow new
    tabs by default.

## 2. What the code does today (checked 2026-10-03)

| # | Finding | Where |
|---|---|---|
| 1, 5 | Three separate selections. Devices: a module-level `Set` (kept while navigating). Checklist: its own `Set`, created per render, rows selected by clicking (Ctrl/Shift), **no checkboxes**. Firmware: its version checkboxes are update choices, not a selection. The *Checklist*/*Charts* buttons pass the selection as `#/checklist?ids=…`; the sidebar link has no `ids`, so it shows everything. | `pages/devices.ts:116`, `pages/checklist.ts:82`, `:70` |
| 6 | With `?ids=` Checklist shows only those devices and has no way back to all of them except the sidebar. | `pages/checklist.ts:112` |
| 7 | The buttons work, but only after **clicking a row** (nothing says rows are clickable; the hint is a muted line). Even then they are active only when *every* selected device has the setting *and* the same value (ShellyScanner's `sameBoolean`/`sameStringOrInt`): LED is Gen1 only, AP Gen2+ only, so a mixed selection leaves most grey without saying why. | `pages/checklist.ts:143`, `checklistlogic.ts` |
| 8 | Both pages ask only above 8 devices (ShellyScanner's threshold). | `pages/devices.ts:169`, `pages/checklist.ts:133` |
| 4 | **Refresh** (`Devices.Refresh`): ask every device in the list for its status now; the list stays. **Rescan** (`Devices.Rescan`): forget the list, keep stored devices as "not found", scan the network again. **Reload** (`Devices.Reload`): for the *selected* devices, identify the device again from its address — type, name, settings, password; the way to retry a device in error or behind a password, or after renaming it on the device. Not superfluous, but its name and tooltip (`Read the device again from its address`) do not say when to use it. | `internal/service/devices.go:169,524,554` |
| 9 | Firmware takes `?ids=` like Checklist; the sidebar shows all; it does not know the Devices selection. | `pages/firmware.ts` |
| 10 | `GET /api/v1/firmware` reads every device on the server and answers once; the panel waits for it. The *Check* button already fetches row by row (`firmwareApi.rows([id])`), so the per-device path exists. | `panels/firmware.ts:96`, `:126` |
| 11, 12 | The integration **does not add Shellys** to Home Assistant; that is the official Shelly integration's job (option B, the discovery handoff, was kept out on purpose — P11-15). Its entities attach to the device with the same MAC (`connections`); without a Shelly-integration device it creates one of its own. So on `ha-test`, without the Shelly integration, every Shelly is a ShellyLanMan-only device. If the Shelly integration *does* have the device and it still shows twice, the MAC match failed (to check on `ha-test`). | `shellylanman-ha` `entity.py` |
| 13 | `default_manufacturer="Shelly"` in `device_info`. `manufacturer="Shelly"` is the same value the Shelly integration writes, so switching changes nothing visible; `default_name`/`default_model` stay (not deprecated, and they must not overwrite names). | `entity.py:28` |
| 15 | Already true: the HA app (`shellylanman-ha/shellylanman`) and the Docker image (`ghcr.io/wimmme/shellylanman`). What is missing is that both READMEs say it on their first screen. | READMEs |
| 16 | Palette `default` = no `data-palette` attribute (the base CSS rules). The HA palette is the start look only under `/api/hassio_ingress/` (P11-13). | `appearance.ts:216`, `app.css:12,39` |
| 17 | `app.css`: < 900 px an icon rail at the left, < 560 px a bottom bar. A phone in portrait is < 560 px, in landscape wider. | `app.css:186–197` |
| 18 | No manifest, no service worker. The CSP (`default-src 'self'`) already allows a same-origin manifest. Browsers install a PWA only in a **secure context**: HTTPS (e.g. via HAProxy, `*.wimmme.net`) or `localhost` — not on `http://192.168.x.x:3082`; there it can only be a home-screen shortcut. Inside Home Assistant the HA app is already the PWA. | `web/public/index.html` |

| 19 | `Devices.Rescan` empties the list and puts every archived device back as *ghost* (label *archived*) at once; a found device replaces its ghost when it answers. mDNS has no "end"; only the IP scan has one (`scan.Scanning`). Archived devices are probed at their last address after 45 s (`retryGhosts`, only with *auto reload*). | `internal/service/devices.go:169–245,468` |
| 20 | *Error* has two causes. (a) A device that could not be identified (unmanaged, type "Generic"): retried every 2 min (`retryErrors`), and by *Reload* and *Refresh*. (b) A known device whose last poll failed with something other than "offline" or "password" (`statusOf`): its own poll loop retries it, *Refresh* triggers that at once, *Reload* identifies it again. So *Reload* on the selected device was the intended fix; that it stayed in error suggests a call that keeps failing on that device (its scripts? flaky Wi-Fi?) — the error text is in the status tooltip. To check on ShellyTestPlug. `Refresh(ids)` already accepts ids on the server; the UI always sends none. | `devices.go:443–464,524,554,646` |
| 21 | English uses ShellyScanner's *on line* / *off line* (`status.online`, `status.offline`, `deferred.intro`); the other languages already write it as one word. | `i18n/en.json:70,71,356` |
| 22 | Four `window.open(…, '_blank')`: Devices, Checklist, the Firmware panel, double-click. No icon. In Home Assistant's panel (HTTPS) a device page (`http://`) cannot load *inside* the frame (mixed content); "same tab" there means leaving Home Assistant for the device page (back button to return). | `pages/devices.ts:172`, `pages/checklist.ts:135`, `panels/firmware.ts:83` |

## 3. Analysis and proposals

### 3.1 One selection for the whole app (items 1, 5, 6, 9)

The core problem is three selections. Proposal: **one shared selection** (a small
`selection.ts` store, kept in `sessionStorage` per tab), used by Devices, Checklist and
Firmware, with one rule for every list page:

- **Scope** — which rows a page shows. Opened with a selection: the selected devices,
  with a banner *"3 selected devices — Show all"*. Opened without: all devices. The
  scope is taken when the page opens and does not shrink while you untick, so a row
  never disappears under the mouse.
- **Selection** — the checkboxes. Checklist gets the same checkbox column, click,
  Ctrl/Shift-click and header checkbox as Devices. Ticking on Checklist changes the
  shared selection, so going back to Devices shows the same ticks.
- **Firmware** keeps its *stable/beta* checkboxes (they choose what to install, not
  which devices); the shared selection only sets its scope.
- `?ids=` in the address stays working (links, Charts), and sets the selection.

This is a deliberate deviation from ShellyScanner, where each dialog had its own table.
Data model and server API do not change.

### 3.2 Devices toolbar (items 2, 3, 4)

Order, in groups separated as on Checklist:

1. *Device selection ▾* · **Checklist** · *Firmware* (new: opens the Firmware page with the selection — optional, Q3)
2. Several devices: *Web UI* · *Settings* · *Charts* · *Backup* · *Restore* · *Read again* (Reload) · *Reboot*
3. One device: *Info* · *Logs* · *Scheduler* · *Scripts* · *Notes*
4. Right: filter, *Columns*, *View*, *CSV*, *Print*, *Refresh*, *Rescan*

Texts (all 8 languages):

- **Refresh** — "Read the current status of every device in the list. The list itself stays as it is."
- **Rescan** — "Search the network again and build the list from scratch. Devices that are not found stay as stored (not found)."
- **Reload** → rename to **Read again** — "Identify the selected devices again from their address: type, name, settings. Use it for a device in error, behind a new password, or after you changed it on the device itself."

### 3.3 Checklist actions (items 7, 8)

With checkboxes (3.1) the "how" is visible. The "why grey" remains. Two levels:

- **a (small, keeps ShellyScanner's rules):** a disabled button gets a tooltip with the
  reason — *"select one or more devices"*, *"not available on Gen2+ devices (LED is
  Gen1)"*, *"the selected devices differ: select devices with the same value"*.
- **b (deviation):** a button is active when the setting *applies* to all selected
  devices; with mixed values it opens a small menu *Turn on / Turn off* instead of
  toggling. Devices it does not apply to are skipped and listed in the result.

Web UI for more than one device always asks: *"This opens N tabs, one per device's
web interface. The browser may block pop-ups."* (Devices and Checklist alike).

### 3.4 Firmware loads like Checklist (item 10)

Draw the rows at once from the device list (name, status, spinner), then fetch each row
with the existing per-device call, six at a time, as Checklist does. The server API
stays; the "Shelly index" column loads as now, after the rows. Small.

### 3.5 Home Assistant (items 11–15)

- Answer to 11: the official Shelly integration adds the Shellys and their switches,
  meters and firmware updates; ShellyLanMan's integration only adds its own status,
  backup and checklist entities to those devices. Adding all Shellys to HA in one go
  from ShellyLanMan is option B, deliberately left out (P11-15); it can be the next
  big HA step, after its own analysis.
- 12: check on `ha-test` whether the Shelly integration is configured there and
  whether a device shows twice (MAC match). Docs: say plainly in both READMEs and the
  integration description that the Shelly integration comes first.
- 13: `default_manufacturer` → `manufacturer` (same value). Test, then a patch release
  of the integration together with the manifest fix that is on `main` (§7 item 6).
- 15: first screen of both READMEs: *two ways to install — Home Assistant app, or Docker*.

### 3.6 Look (item 16)

Make the start look everywhere: *Home Assistant* palette, light or dark following the
system until the user chooses (today only inside HA). Stored choices are kept, so only
browsers that never chose change. Rename the *Default* palette, proposals (Q5):
**Midnight** (it is a deep blue-black; its light variant "Midnight light"),
*Classic*, or *MikroDash* (where the look came from). The internal id becomes the new
name, with a one-time migration of a stored `default`.

### 3.7 Phone navigation (item 17)

Proposal: the icon rail at the left on every width (52 px, fixed, does not scroll
with the page), no bottom bar. On a 360–390 px phone that leaves ~310 px for the page;
tables already scroll sideways. Alternative: a collapsible menu (☰) — more code, and
one tap more for every page change.

### 3.8 PWA (item 18)

A web app manifest (name, icons 192/512 + maskable, `display: standalone`, theme
colour) and the existing apple-touch icon. No service worker with caching: the app is
useless offline and a cache risks an old UI after an update (Chrome no longer requires
a service worker to install). Works where the page is served over HTTPS or on
localhost; on plain `http://<ip>:3082` only as a home-screen shortcut — the docs must
say so. Inside Home Assistant not needed. Small.

### 3.10 Rescan without the "everything archived" moment (item 19)

New status **searching** (grey, spinner): on *Rescan* the devices that were in the list
keep their row with that status. Each is probed at once at its last address (what
`retryGhosts` does after 45 s, now immediately and for every known device), while
mDNS / the IP scan runs. Found → online as now. Not found after a search window (IP
scan: when the scan ends; mDNS: 30 s) → *archived* (or removed when the archive is
off). Server change in `Devices.Rescan` with tests; the UI gets one status more.
Deviation from ShellyScanner (which shows the ghosts at once).

### 3.11 Refresh, Rescan and a device in error (item 20)

- **Refresh follows the selection:** with devices selected, *Refresh* reads only those
  and says so (*Refresh 1*); without, all. The server supports it already.
- **Rescan stays global** and is not dimmed (dimming because of a selection is hard to
  understand: "why can't I rescan?"). Its label and tooltip say *Scan the network again*.
- **Error is explained in place:** the status tooltip shows the error text already;
  add one line *"Select the device and press Read again"*. A device in error is also
  read again on *Refresh* (as now).
- **Investigate** why ShellyTestPlug stayed in error (read-only: log of the live
  container, the device's answers with `cmd/record`). If one optional call (scripts,
  a component) puts a whole device in error, that is a bug to fix: the device is
  online, the failing part is shown as missing.

### 3.12 "online" and Web UI links (items 21, 22)

- English: *online* / *offline* (also `deferred.intro`). Trivial.
- Every link that leaves ShellyLanMan gets the ↗ icon (Web UI buttons, the menu item,
  release notes, About links).
- New per-browser setting (Settings → Appearance, stored like the theme, because a
  wall tablet and a desktop differ): **Open device pages — in a new tab / in this
  tab**. Default: new tab. "This tab" replaces ShellyLanMan (inside Home Assistant:
  the whole HA window) with the device page. With more than one device "this tab" can
  open only one, so the multi-device Web UI is then disabled with that reason. Fully
  Kiosk users may alternatively allow pop-ups in Fully's own settings — the help text
  says so.

### 3.13 Documentation and positioning

- `docs/brief.md` rule "nothing beyond ShellyScanner without approval" stays; this
  phase is that approval for the page logic (new decision P12-1).
- `FEATURE_PARITY.md`: the affected rows get a note "deliberately different (P12-x)".
- About page and READMEs: "Started from ShellyScanner and built on its basis, then
  developed further as a web application" — replaces the planned "Inspired by"
  (§7 item 2 of phase 11), GPL attributions unchanged. Screenshots (§7 item 3) after
  this phase, so they show the new pages.

## 4. Open work carried over (phase 11 §7)

1. Push of local commits — still to be asked.
2. Wording about ShellyScanner — now §3.13; Q7.
3. Screenshots — after this phase.
4. Text review — after this phase.
5. HACS default list, forum post — later.
6. Manifest key order in `shellylanman-ha` — in the patch release with 3.5.
7. Testing 0.5.0 — this feedback is its result.
8. Dev volume on dockerhostvm; option B — unchanged.

## 5. Questions for Wim

1. **Selection model (3.1):** one shared selection with *scope taken when the page
   opens* — agreed? Or should unticking on Checklist/Firmware hide the row at once?
2. **Checklist buttons (3.3):** level a (explain why grey), or b (active when the
   setting applies; mixed values → On/Off menu)?
3. **Firmware button** on the Devices toolbar (3.2), next to Checklist?
4. **Reload** → rename to *Read again* and keep, as proposed?
5. **Palette name** for today's Default: Midnight, Classic, MikroDash, or yours?
6. **"ShellyLanMan app → dit zou wel handig zijn"**: what is meant — a link from the
   integration to the app panel, a ShellyLanMan entry in the sidebar also for the
   Docker version, something in the app log, or something else?
7. **ShellyScanner wording:** "started from / based on, developed further" — can that
   go ahead now, or still wait for the developer's answer?
8. **Phone (3.7):** fixed icon rail at the left, or a ☰ menu?
9. **Rescan (3.10):** *searching* first, archived after the search window — agreed,
   and is 30 s for mDNS right?
10. **Refresh/Rescan (3.11):** Refresh follows the selection, Rescan stays global and
    active — agreed (instead of dimming Rescan)?
11. **Web UI (3.12):** per-browser setting new tab / this tab, default new tab?

## 6. Plan

Each step its own commits, with tests and `CHANGELOG.md`; all strings in 8 languages.

| Step | What | Size | Needs |
|---|---|---|---|
| 12.1 | Docs: DECISIONS P12-x for the answers, FEATURE_PARITY notes | S | §5 answers |
| 12.2 | Shared selection store; Devices uses it; Checklist and Firmware scope + *Show all* banner | M | Q1 |
| 12.3 | Checklist checkbox column, header checkbox; Web UI confirm for > 1 on both pages | S–M | 12.2 |
| 12.4 | Checklist button states (a or b) | S (a) / M (b) | Q2 |
| 12.5 | Devices toolbar groups and order; Refresh/Rescan/Read again texts | S | Q3, Q4 |
| 12.6 | Firmware rows at once, filled per device | S | — |
| 12.7 | HA palette as start look everywhere; rename Default palette | S | Q5 |
| 12.8 | Phone navigation | S | Q8 |
| 12.9 | PWA manifest and icons; docs on HTTPS | S | — |
| 12.10 | `shellylanman-ha`: `manufacturer`, ha-test check of item 12, README first screen (two install ways, Shelly integration first); patch release on Wim's go | S | Q6 |
| 12.10a | Rescan with *searching* (server + UI) | M | Q9 |
| 12.10b | Refresh follows selection; error hint; investigate ShellyTestPlug's error (read-only) and fix if it is a bug | S–M | Q10 |
| 12.10c | *online*; ↗ on external links; setting new tab / this tab | S | Q11 |
| 12.11 | Wording About/READMEs (§3.13), then screenshots and text review (phase 11 §7 items 2–4) | M | Q7 |
| 12.12 | Release 0.6.0 on Wim's go; Wim tests on phone, desktop and `ha-test` | — | — |

Verification: `sh tools/verify.sh` (remote on dockerhostvm), the simulator for the
pages, no device writes needed except a Checklist action on Grondwaterpomp /
ShellyTestPlug to show the buttons work (asked first).

## 7. Answers (2026-10-03) — DECISIONS §21, P12-1…P12-14

1. Agreed. 2. Option **a**: keep the rules, explain in the tooltip. 3. Firmware button
if there is room. 4. Keep *Reload*; a clear tooltip is enough. 5. **Midnight**
(MikroDash names it plainly "Default Dark / Default Light"). 6. It was a line from
another app's log; ShellyLanMan should log its successful Home Assistant discovery
announcement too (`cmd/shellylanman/main.go` logs only failures today). 7. Go ahead
now — it makes it easier to present to ShellyScanner's developer. 8. **☰ menu**: there
is too little horizontal room for a rail. 9. Agreed. 10. Agreed; the principle for the
whole app is *enough explanation in the tooltip*. 11. Agreed.

Plan changes: 12.5 keeps the name *Reload*; 12.8 becomes a ☰ menu; 12.10 adds the
discovery log line (main repo) next to the integration fix; 12.11 can start any time;
a tooltip pass over every page is part of each step (P12-4).

## 8. Progress (2026-10-03)

| Step | Commit | Notes |
|---|---|---|
| 12.1 | `ddfaa36` | `FEATURE_PARITY.md` §6 and "P12 §6" on the rows |
| 12.2, 12.3 | `f60cdfb` | `web/src/selection.ts` (sessionStorage per tab); scope banner; Checklist checkboxes; Web UI confirm > 1 |
| 12.4 | `bce11c8` | `web/src/why.ts`: tooltip says why a button is grey |
| 12.5 | `318b4c9` | Devices toolbar groups, Firmware button, Refresh follows the selection |
| 12.6 | `9400b8e` | Firmware rows at once, six devices at a time |
| 12.7 | `7c9ed62` | HA palette start look everywhere; *Default* → *Midnight* (stored `default` migrated) |
| 12.8 | `ab83014` | ☰ drawer below 900 px |
| 12.9 | `51cd1a4` | `manifest.json`, icons 192/512 from `docs/images/ShellyLanManLogoSmall.png` |
| 12.10a | `020a377` | status `searching`; probes after 3 s; end of search: IP scan end + probe time, mDNS 30 s |
| 12.10b | `acd978a` | **cause of item 20 found on `ha-test`**: ShellyTestPlug was listed as `addr:192.168.0.150:80` (unidentified, error) next to the identified device; that row now goes when the device is identified |
| 12.10c | `cbe2c0c` | `web/src/weblinks.ts`; ↗ via CSS on `a[target=_blank]` and `.btn.ext` |
| 12.10 | `20c2a3a`, `99b32f1`; `shellylanman-ha` `b44633c` | discovery success logged; integration: `name`/`manufacturer`/`model` (all `default_*` deprecated), no device for `addr:` ids (0.5.0 leftovers removed), status `searching` |
| 12.11 | `a71a626`; `shellylanman-ha` `58da6a9` | wording on About (8 languages) and READMEs |

**Item 12 explained (checked on `ha-test`, HA 2026.9.4, source `helpers/device_registry.py`):**
since Home Assistant 2026.8 a device belongs to one config entry ("Version 3 restricts a
device to a single config entry"); devices of different integrations with the same MAC
are no longer merged but shown as linked. ShellyLanMan's device for BrandstofcelSwitch
and the Shelly integration's are therefore two linked devices — by design of Home
Assistant, not a bug in the integration.

Open: screenshots / text review (phase 11 §7 items 3–4); `shellylanman-ha` GitHub
description still says "(and later integration)" — change only with Wim's go; release
0.6.0 of both on Wim's go.
