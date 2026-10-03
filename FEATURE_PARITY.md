# ShellyLanMan — Feature parity with ShellyScanner

> Status: **all rows implemented or agreed as a deviation (Phases 1–10, v0.2.0).** A row is ticked (✅) only when it is
> implemented **and** covered by tests (unit + simulator, and real hardware where it touches devices).
>
> Reference: ShellyScanner 1.3.4, commit `a8e9b93` (2026-09-17). Java paths are relative to
> `src/main/java/it/usna/shellyscan/`. "G1/G2/G3/G4/BLU" = generations. "Phase" refers to the plan in
> `DECISIONS.md` §6.

Status legend: `—` not started · `🔨` in progress · `✅` done and tested · `⚠️` deviation agreed (see notes)
· `❓` needs a decision (see `DECISIONS.md` §8).

---

## 1. Feature matrix

### 1.1 Discovery, identification, device list

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| D1 | Full mDNS scan (all interfaces), follows interfaces appearing/disappearing | `model/Devices.scannerInit(boolean…)`, `JmmDNS`, `MDNSListener` | mDNS `_http._tcp.local.` browse + resolve, then `GET /shelly` | G1–G4 | Discovery service; "Full mDNS" scan mode; host networking required (see ARCHITECTURE §2.5) | 2 | ✅ |
| D2 | Local mDNS scan (one interface: the local host address) | `Devices.scannerInit`, `Devices.rescan` (re-bind if local IP changed) | same | G1–G4 | "Local mDNS" mode = **interface picker** (full = all interfaces, or one chosen interface) — decided Q13 — P9: checked on hardware (eth0 on dockerhostvm, 25 instances) | 2 | ✅ |
| D3 | IP scan, up to 10 ranges `a.b.c.first-last` | `Devices.scanByIP`, `IPCollection`, `view/appsettings/PanelNetwork`, `DialogNetworkIPScanSelection` | `GET http://ip:80/shelly` (recognised by `mac`) | G1–G4 | IP-scan mode with range editor; works in bridge networking ⚠️ no ICMP pre-ping (see §4.2) | 2 | ✅ |
| D4 | Offline mode: no scan, archive only | `Main` (`OFFLINE`, `-noscan`), `IPCollection` empty | none | all | "Offline" scan mode | 2 | ✅ |
| D5 | Device identification by generation and type/app/model | `model/DevicesFactory.create*` | `/shelly`: `gen`, `type`, `app`, `model`, `svc0.type`, `mac`, `auth`/`auth_en` | G1–G4 | Model registry (§2) | 2 | ✅ |
| D6 | Unknown/failed devices shown as "unmanaged" (Generic G1/G2/G3/G4) and retried 30 s after start | `Shelly*Unmanaged`, `ShellyGenericUnmanagedImpl`, `Devices.errorsReconnect` | `/shelly`, then gen-specific | G1–G4 | Same states; retry job — P9: same retry rule as the original (O33) | 2 | ✅ |
| D7 | mDNS host named `shelly*` whose `/shelly` fails is still listed (with error) | `Devices.create(…, force)` | — | G1–G4 | Same | 2 | ✅ |
| D8 | Range-extender clients discovered as `ip:port` | `Devices.create` (range extender block), `g2/modules/RangeExtenderManager` | `WiFi.ListAPClients` → ports, `/shelly` on each port | G2+ | Same; address = IP + port — P9: simulator test (no range extender here) | 2 | ✅ |
| D9 | BLU devices discovered through gateways (BTHome devices, BLU TRV), duplicates across gateways merged, alternative parents shown in tooltip | `Devices.create` (BTHome block), `Devices.newBluDevice`, `DevicesFactory.createBlu`, `blu/*`, `BluInetAddressAndPort` | `Shelly.GetComponents?dynamic_only=true` (paged); keys `bthomedevice:*`, `blutrv:*` | BLU via Pro/G3/G4 gateways | Same — P9: simulator test (no BLU devices here) | 2 | ✅ |
| D10 | Same device (MAC) at a new address replaces the old row; a recognised device is not replaced by an unmanaged one | `Devices.newDevice` | — | all | Same rule in service | 2 | ✅ |
| D11 | Rescan (clear list, scan again, re-add archive ghosts) | `MainView.rescanAction`, `Devices.rescan` | as D1–D3 | all | Toolbar "Rescan" | 2 | ✅ · P12 §6 |
| D12 | Refresh all (status icon → "reading", refresh each non-ghost) | `MainView.refreshAction`, `Devices.refresh` | `refreshSettings` + `refreshStatus` | all | Toolbar "Refresh" | 2 | ✅ · P12 §6 |
| D13 | Reload / Login single device (recreate from address; asks credentials if needed) | `MainView.reloadAction`, `Devices.create(…, false)` | `/shelly` + init | all | Row action "Reload"/"Login" | 2 | ✅ |
| D14 | Online / offline / not logged / reading / error / ghost status, reboot-required variant | `ShellyAbstractDevice.Status`, `DevicesTable` icons | derived from HTTP results; `sys.restart_required` (G2+) | all | Status pill column | 2 | ✅ |
| D15 | Periodic refresh: status every N s (default 2), config every M status refreshes (default 5), per-device staggering | `Devices.scheduleRefresh`, settings `REFRESH_INTERVAL`, `REFRESH_SETTINGS` | G1 `/status`, `/settings`; G2+ `Shelly.GetStatus`, `Shelly.GetConfig`; BLU via gateway (`Shelly.GetComponents?keys=…`, `BluTrv.Get*`) | all | Poller | 2 | ✅ |
| D16 | Pacing: ~59 ms between calls to the same device; max 8 connections per device | `Devices.MULTI_QUERY_DELAY`, `HttpClient.setMaxConnectionsPerDestination(8)` | — | all | Poller/client pacing | 1–2 | ✅ |
| D17 | Ghost auto-reload 45 s after start (non-battery, non-BLU ghosts probed at last address) | `Devices.ghostsReconnect`, setting `AUTORELOAD` | `/shelly` | G1–G4 | Same | 2 | ✅ |
| D18 | Credentials for protected devices: prompted once, reused for the next devices; optionally stored in settings | `DevicesFactory.setCredential`, `digestAuthentication`, `DialogAuthentication`, `PanelNetwork` (RLUSER/RLPWD) | G1 Basic auth; G2+ digest SHA-256 (user `admin`) | G1–G4 | Devices appear as "not logged"; Login modal; credentials stored encrypted, **per device with a global default** (Q9) ⚠️ no blocking prompt in a server | 2 | ✅ ⚠️ |

### 1.2 Device table (main view)

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| T1 | Columns: Status, Type, Device (hostname), Name, Keyword, MAC, IP, SSID, RSSI (dBm), Cloud (En/Con), MQTT (En/Con), Uptime, Temp, Measurements, Logs, Source, Command | `view/MainView` (`tabModel`), `view/DevicesTable` (COL_*), `updateRow` | from D15 data | all | Devices table page with the same columns and headings | 3 | ✅ |
| T2 | Default hidden columns: Keyword, MAC, SSID, Logs | `DevicesTable.loadColPos` | — | — | Same defaults | 3 | ✅ |
| T3 | Column chooser & order, separately for "default" and "detailed" view; reset on table-version change | `DevicesTable.saveColPos/loadColPos`, `DialogAppSettings` (Columns), `Main.TAB_VERSION` | — | — | Column menu; stored per browser | 3 | ✅ |
| T4 | Detailed / default view toggle (window resize modes: full, horizontal, estimate, as-is) | `MainView.detailedView`, setting `DETAIL_SCREEN` | — | — | View-mode toggle; resize modes are not applicable in a browser ⚠️ (§4.1) | 3 | ✅ |
| T5 | Sorting per column (IP numeric, measurements, command label, source) | `DevicesTable` comparators, `IPv4Comparator` (usnalib2) | — | — | Sortable headers; same comparators | 3 | ✅ |
| T6 | Filter text on All / Type / Device / Name / Keyword; default filter column setting; Ctrl+F focus, Ctrl+S cycle column, Ctrl+E clear | `MainView.setColFilter`, `DevicesTable.setRowFilter`, setting `DEFAULT_FILTER` | — | — | Filter box + column select; keyboard shortcuts | 3 | ✅ |
| T7 | Selection helpers: all, online, reboot required, Gen1, Gen2+, Wi-Fi, BLU, stored (ghost) devices; Ctrl+Shift subtract | `MainView` (SelectionAction, `UsnaDropdownAction`) | — | — | Selection menu | 3 | ✅ · P12 §6 |
| T8 | Status line: "N devices listed – M selected" / filtered variant | `MainView.displayStatus` | — | — | Summary cards + footer count | 3 | ✅ |
| T9 | Tooltips: uptime (d/h/m/s + since), status description, BLU parents, G1 TRV profile/target/position, measurements table | `DevicesTable.getToolTipText` | — | — | Tooltips / expandable cells | 3 | ✅ |
| T10 | Uptime format: seconds / days-hh-mm-ss / since date | `UptimeCellRenderer`, setting `UPTIME_MODE` | `uptime` | all | Same setting | 3 | ✅ |
| T11 | Temperature unit °C/°F (table, meters, command column) | `FahrenheitTableCellRenderer`, setting `TEMP_UNIT` | — | — | Same setting | 3 | ✅ |
| T12 | Measurements column: typed meters (P, Q, S, pf, V, I, f, T, H, L, bat, …) with labels and names | `DeviceMetersCellRenderer`, `meters/*`, per-model `getMeters()` | status payloads | all | Measurements cell | 3 | ✅ |
| T13 | Source column (last input event source) | `DevicesTable` COL_SOURCE, `col_last_source_tooltip` | status payloads | G2+/BLU | Same | 3 | ✅ |
| T14 | Copy cell / hostname / MAC (Ctrl+C on a cell) | `ExTooltipTable.activateSingleCellStringCopy` (usnalib2) | — | — | Copy buttons / selection copy | 3 | ✅ |
| T15 | Double-click action: device info or open Web UI (setting) | `MainView` mouse listener, setting `DCLICK_ACTION` | — | — | Row click opens detail panel; setting kept | 3 | ✅ |
| T16 | Context menus (device / ghost) | `MainView` `tablePopup`, `ghostDevPopup` | — | — | Row action menu — P9: row menu added (it was missing in Phase 3) | 3 | ✅ |
| T17 | Toolbar captions on/off | setting `T_CAPTIONS` | — | — | Not needed (icons + labels responsive) ⚠️ | 3 | ⚠️ |
| T18 | Print table | `MainView.printAction` | — | — | Browser print with print stylesheet | 8 | ✅ |
| T19 | Export table as CSV (visible columns, configurable separator) | `controller/ExportCSVAction`, setting `CSV_SEPARATOR` | — | — | "Export CSV" (download) | 8 | ✅ |
| T20 | Open device Web UI (confirm if > 8) | `MainView.browseAction` | — | G1–G4 | Link opening `http://ip[:port]` in a new tab (browser must reach the device) ⚠️ | 3 | ✅ · P12 §6 |

### 1.3 Read-only device information

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| I1 | Device info dialog: one tab per info request, raw JSON, auto-updates when device comes online, keyboard tab navigation | `view/DialogDeviceInfo`, `getInfoRequests()` per class | G1: `/shelly`, `/settings`, `/settings/actions`, `/status`; G2+: `Shelly.GetDeviceInfo?ident=true`, `Shelly.GetConfig`, `Shelly.GetStatus`, `Shelly.CheckForUpdate`, `Schedule.List`, `Webhook.List`, `Script.List`, `WiFi.ListAPClients`, `KVS.GetMany`, `Shelly.GetComponents`, `BLE.CloudRelay.ListInfos` (+ model extras: `Matter.*`, `XMOD.*`, `KNX.GetConfig`, `SensorAddon.GetPeripherals`, `EM*Data.*`…); battery G2: subset; BLU: `BTHomeDevice.*`, `BTHomeSensor.*`, `BluTrv.*` | all | Device detail panel with tabs and a JSON viewer — find = the browser's Ctrl+F | 3 | ✅ |
| I2 | Battery devices: last stored JSON shown when asleep ("stored data used") | `BatteryDeviceInterface`, `AbstractBattery*Device.getStoredJSON` | — | G1/G2+ battery | Same (kept in memory) | 3 | ✅ |
| I3 | Logs Gen1: `/debug/log`, `/debug/log1` snapshot | `view/DialogDeviceLogsG1` | `/debug/log`, `/debug/log1` | G1 | Logs panel | 3 | ✅ |
| I4 | Logs Gen2+: live stream via WebSocket (auth via query params on protected devices), BLU → parent's log | `view/DialogDeviceLogsG2`, `AbstractG2Device.connectWebSocketLogs` | `ws://host/debug/log` | G2+, BLU | Server proxies the device WS to the browser WS | 3 | ✅ |
| I5 | Uptime, RSSI, SSID, cloud, MQTT, temperature, meters, reboot required | per-model `fillStatus/fillSettings` | see D15 | all | Table + detail | 3 | ✅ |

### 1.4 Device controls ("Command" column)

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| C1 | Relay on/off/toggle (multi-channel), input state indicator | `DevicesCommandCellEditor/Renderer`, `RelayInterface`, `g1/modules/Relay`, `g2/modules/Relay` | G1 `/relay/<n>?turn=on|off|toggle`; G2+ `Switch.Set`, `Switch.Toggle` | G1–G4 | Toggle switch(es) in cell | 4 | ✅ |
| C2 | Roller/cover open/close/stop/position, calibrated flag | `RollerInterface`, `g1/modules/Roller`, `g2/modules/Roller` | G1 `/roller/<n>?go=open|close|stop|to_pos&roller_pos=`; G2+ `Cover.Open/Close/Stop/GoToPosition` | G1–G4 | Buttons + slider | 4 | ✅ |
| C3 | White light on/off/brightness (min brightness), input state | `WhiteInterface`, `g1/modules/LightWhite`, `g2/modules/LightWhite` | G1 `/light/<n>` or `/white/<n>?turn=&brightness=`; G2+ `Light.Set`, `Light.Toggle` | G1–G4 | Toggle + slider | 4 | ✅ |
| C4 | RGB / RGBW / CCT / RGBCCT (colour, gain, white, temperature, colour mode) with editor dialog | `RGBInterface`, `RGBWInterface`, `CCTInterface`, `RGBCCTInterface`, `view/lightsEditor/*`, `g1/modules/LightRGBW`, `LightBulbRGB`, `g2/modules/LightRGB(W)`, `LightCCT`, `g3/modules/LightRGBCCT` | G1 `/color/<n>`, `/light/<n>?mode=&red=&green=&blue=&white=&gain=&temp=`; G2+ `RGB.Set`, `RGBW.Set`, `CCT.Set`, `RGBCCT.Set` | G1–G4 | Light popover/modal | 4 | ✅ |
| C5 | Thermostat enable / target temperature (G1 TRV profile & position tooltip) | `ThermostatInterface`, `g1/modules/ThermostatG1`, `g2/modules/ThermostatG2`, `g3/modules/XT1Thermostat`, `blu/BluTRV` | G1 `/settings/thermostats/0?…`, `/thermostat/0?target_t=`; G2+ `Thermostat.SetConfig` / `Thermostat.*`; XT1 `Number.Set`/`Boolean.Set` (service components); TRV `BluTrv.Call` | G1, G2+, BLU | Stepper + toggle | 4 | ✅ |
| C6 | Input: show state; execute an input's configured actions (G1 action URLs / G2+ webhooks called **by the scanner**) | `InputInterface`, `g1/modules/Actions.execute`, `g2/modules/Webhooks.execute`, `g2/modules/Input` | G1 action URLs; G2+ webhook URLs (`127.0.0.1` rewritten to device IP); `Input.trigger` exists in the model but no view calls it | G1–G4, BLU | Input event buttons; server performs the GETs ⚠️ note (§5) | 4 | ✅ ⚠️ |
| C7 | Circuit breaker toggle with confirmation, lock state | `CBreakerInterface`, `g2/modules/CBreakerPro` | `CB.Set`, `CB.GetLog` | Pro 2CB (prototype) | Toggle + confirm | 4 | ✅ |
| C8 | Camera privacy on/off | `CameraInterface`, `g3/modules/Camera` | `Camera.Set` (+ zones: `Camera.AddZone/DeleteZone`, `CameraZone.SetConfig`) | G3 Camera | Toggle | 4 | ✅ |
| C9 | Sensor states in cell: motion, flood, smoke, door/window, presence zones | `MotionInterface`, `FloodInterface`, `SmokeInterface`, `DWInterface`, `PresenceZoneInterface` | status payloads | G1–G4, BLU | Read-only badges | 3 | ✅ |
| C10 | Reboot selected devices (confirmation; BLU only TRV) | `MainView.rebootAction`, `Devices.reboot` | G1 `/reboot`; G2+ `Shelly.Reboot`; TRV `BluTrv.call?method=Shelly.Reboot` | G1–G4, TRV | Toolbar/row action + confirm modal | 4 | ✅ |

### 1.5 Configuration ("Devices conf." dialog, multi-device)

All tabs apply to the selection, show per-device results (success / fail / fail+reason / excluded /
queued / cancelled) and exclude devices that are not applicable or offline (`dlgExcludedDevices*`).
Where marked "deferrable", an offline device gets a queued task (see §1.7).

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| S1 | FW Update tab (see §1.6) | `view/devsettings/PanelFWUpdate`, `FWUpdateTable` | see F* | all | "Firmware" tab / page | 7 | ✅ |
| S2 | Wi-Fi 1 and Wi-Fi 2: keep / disable / DHCP / static (IP, netmask, gateway, DNS), SSID+password, validation, confirm warning; copy from another device | `view/devsettings/PanelWIFI`, `g1/modules/WIFIManagerG1`, `g2/modules/WIFIManagerG2` | G1 `/settings/sta`, `/settings/sta1?enabled=&ssid=&key=&ipv4_method=&ip=&netmask=&gw=&dns=`; G2+ `WiFi.SetConfig {sta|sta1}` (`Wifi.GetConfig`) | G1–G4 | Wi-Fi tabs | 5 | ✅ |
| S3 | Restricted login enable/disable (user, password), deferrable | `view/devsettings/PanelResLogin`, `LoginManagerG1/G2` | G1 `/settings/login?enabled=&username=&password=`; G2+ `Shelly.SetAuth {user:"admin", realm, ha1}` | G1–G4 | Login tab | 5 | ✅ |
| S4 | MQTT: enable/disable, server, user/password, custom or default prefix; G1 extras (reconnect timeouts, clean session, keep-alive, QoS, retain, update period); G2+ extras (RPC notifications, generic status, MQTT control, RPC over MQTT); mixed selection panel; copy; "-slow" delay; deferrable | `view/devsettings/PanelMQTTG1`, `PanelMQTTG2`, `PanelMQTTMix`, `MQTTManagerG1/G2` | G1 `/settings/mqtt?…`; G2+ `MQTT.SetConfig` | G1–G4 | MQTT tab (variant by selection) | 5 | ✅ |
| S5 | Others: NTP server, Cloud enable/disable, Input reset ("factory reset from switch") enable/disable; all deferrable | `view/devsettings/PanelOthers`, `TimeAndLocationManager*`, `InputResetManager*` | G1 `/settings?sntp_server=`, `/settings/cloud?enabled=`, `/settings?factory_reset_from_switch=`; G2+ `Sys.SetConfig {sntp}`, `Cloud.SetConfig`, `Input.SetConfig {factory_reset}` | G1–G4 | Others tab | 5 | ✅ |
| S6 | Checklist view: per device Eco, LED off, Logs, BLE, AP, Roaming, Wi-Fi 1/2 (static ✓ / DHCP ✗), Extender, Scripts, Auto FW; row actions to toggle each; edit Wi-Fi; multi-selection; filter; refresh; help link | `view/checklist/CheckListView`, `CheckListTable`, `DialogWiFiDevicesInfo`, `DialogBluDevicesInfo` | G1 `/settings`; G2+ `Shelly.GetConfig`, `Shelly.GetStatus`, `WiFi.ListAPClients`, `BLE.CloudRelay.ListInfos`, `Script.List`; writes: `/settings?eco_mode_enabled=`, `/settings?led_status_disable=`, `/settings?debug_enable=`, `/settings?ap_roaming_*`, `Sys.SetConfig {device.eco_mode | debug.*}`, `BLE.SetConfig`, `WiFi.SetConfig {ap | roam | range_extender}`, `Sys.SetConfig {sys.device…}` auto-update | G1–G4, BLU | Checklist page | 5 | ✅ ⚠️ · P12 §6 |
| S7 | Scheduler Gen2+ (cron editor with sunrise/sunset, method hints and parameter editors, test button, load/save) | `view/scheduler/gen2plus/*`, `AbstractCronPanel`, `CronUtils`, `MethodHints`, `g2/modules/ScheduleManager` | `Schedule.List/Create/Update/Delete/DeleteAll` | G2+ (non-battery) | Scheduler modal | 8 | ✅ |
| S8 | Scheduler Wall Display thermostat (profiles: list/rename/delete; rules) | `view/scheduler/walldisplay/*`, `g2/modules/ScheduleManagerThermWD` | `Thermostat.Schedule.ListProfiles/RenameProfile/DeleteProfile/ListRules/UpdateRule/DeleteRule/SetConfig`, `Thermostat.Schedule` | Wall Display | Scheduler modal (WD variant) | 8 | ✅ |
| S9 | Scheduler BLU TRV | `view/scheduler/blutrv/*`, `blu/modules/ScheduleManagerTRV` | `BluTrv.Call` (`TRV.ListScheduleRules`, …) | BLU TRV | Scheduler modal (TRV variant) | 8 | ✅ |
| S10 | Scripts: list, run/stop, enable, create, delete, upload from file (.js or from a .sbk), download, IDE editor (tab size, font, auto-indent, auto-close, dark mode, bracket matching, block comment, find/replace, autocomplete), save as | `view/scripts/ScriptsPanel`, `view/scripts/ide/*`, `g2/modules/Script`, settings `IDE_*` | `Script.List/GetCode/PutCode/Create/Delete/Start/Stop/SetConfig/GetConfig` | G2+ | Scripts modal with code editor | 8 | ✅ |
| S11 | KVS: list (paged), edit value, add, delete (confirm) | `view/scripts/KVSPanel`, `g2/modules/KVS` | `KVS.GetMany` (paged), `KVS.Set`, `KVS.Delete` | G2+ | KVS tab | 8 | ✅ |
| S12 | Notes and keyword per device (stored in archive), delete warning | `view/NotesEditor`, `GhostDevice.note/keyNote`, `DevicesStore` | — | all | Notes modal; Keyword column | 8 | ✅ |
| S13 | Model-specific settings used by restore (profiles, UI settings, add-ons, EM, sensors, LoRa, LNM, Matter, Zigbee, KNX…) | per-model `restore(...)` in `g2/*`, `g3/*`, `g4/*`; `DynamicComponents`, `SensorAddOn*`, `LoRaAddOn` | `Shelly.SetProfile`, `*_UI.SetConfig`, `SensorAddon.*`, `EM.SetConfig`, `EM1.SetConfig`, `PM1.SetConfig`, `Temperature/Humidity/Illuminance/Voltmeter.SetConfig`, `LoRa.SetConfig`, `LNM.*`, `Matter.SetConfig`, `Zigbee.SetConfig`, `Virtual.Add/Delete`, `BTHome.AddSensor/DeleteSensor`, `Service.SetConfig`, … | G2+ | Part of restore (§1.8) | 6 | ✅ |

### 1.6 Firmware

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| F1 | Check current / new stable / new beta per device (device asks the Shelly cloud itself) | `g1/modules/FirmwareManagerG1`, `g2/modules/FirmwareManagerG2`, `blu/modules/FirmwareManagerTRV` | G1 `/ota/check` then `/ota` (`status`, `old_version`, `has_update`, `new_version`, `beta_version`); G2+ `Shelly.CheckForUpdate` + `Shelly.GetDeviceInfo` (`fw_id`, `ver`); TRV `BluTrv.GetRemoteDeviceInfo`, `BluTrv.CheckForUpdates` | G1–G4, TRV | Firmware page/tab: table Device · Current FW · New stable · New beta | 7 | ✅ · P12 §6 |
| F2 | Short version parsing from build IDs (`20241031-171026/2.3.0-…-beta4-QA` → `2.3.0-beta4`) | `FirmwareManager.getShortVersion` (`VERSION_PATTERN`) | — | all | Same regex, with its examples as unit tests | 7 | ✅ |
| F3 | Update to stable or beta, per device choice (checkboxes), select all/none, counters, search, confirmation "Update N devices?" | `PanelFWUpdate` | G1 `/ota?update=true` / `/ota?beta=true`; G2+ `Shelly.Update {stage}`; TRV `BluTrv.UpdateFirmware?id=` (blocking) | G1–G4, TRV | Same | 7 | ✅ |
| F4 | Progress: "load N%", "rebooting", back online detection (uptime drop / status change) | `PanelFWUpdate.FMUpdateListener`, `wsEventListener` | WS `NotifyEvent`: `ota_progress`, `ota_success`, `scheduled_restart` (component `sys` or BLU key) | G2+ (G1: status polling) | Progress events over our WS | 7 | ✅ |
| F5 | Offline / battery devices: "any" update queued as deferred task; stored data used for current version | `PanelFWUpdate` (`dc.addOrUpdate … FW_UPDATE`), battery `getStoredJSON` | as F3 when back online | all | Same | 7 | ✅ |
| F6 | Auto firmware update setting (stable / beta / none) from checklist | `CheckListView.autoFWUpdateAction` | G2+ `Sys.SetConfig` (auto-update) — to be confirmed per fw | G2+ | Checklist action | 5 | ✅ |
| F7 | **NEW**: QR code / local download of the latest stable firmware for a device, served and cached by ShellyLanMan | — (not in original) | server fetches Gen1 index `https://api.shelly.cloud/files/firmware`, Gen2+ index `https://updates.shelly.cloud/update/<app>`; device-side option `/ota?url=` (G1) / `Shelly.Update {url}` (G2+) | G1–G4 (not BLU) | QR + local download via ShellyLanMan, **stable only**, plus server-side version comparison with the Shelly index; no push-URL update (Q17) — `DECISIONS.md` §4.4 | 7 | ✅ |

### 1.7 Deferred actions

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| Q1 | Actions on offline/not-logged/ghost devices are queued and run automatically when the device comes back online; types: FW_UPDATE, RESTORE, BACKUP, MQTT, LOGIN, NTP, CLOUD_ENABLE, INPUT_RESET_ENABLE; one per device+type (replace) | `controller/DeferrableTask`, `DeferrablesContainer` (listens to model UPDATE events) | as the original action | all | Deferred service | 5 | ✅ |
| Q2 | Deferred list: time, device, action, status (waiting/running/success/fail/cancelled), return message, cancel; status-bar button with waiting count and success/fail icon | `view/DialogDeferrables`, `MainView` (btnshowDeferrables) | — | — | "Deferred" page + sidebar badge | 5 | ✅ |
| Q3 | Queue is memory-only (lost on exit) | `DeferrablesContainer` | — | — | **Persisted** in `/data/deferred.json`, secrets encrypted (Q10) | 5 | ✅ ⚠️ |

### 1.8 Backup and restore

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| B1 | Backup single device to `<hostname>.sbk` (ZIP) or many to a folder; progress in status line; result dialog (success / stored data / queued / fail) | `controller/BackupAction` | — | all | Backup action → files in `/data/backups`, downloadable; results modal; **retention: N per device, configurable, default 10** (Q12) | 6 | ✅ |
| B2 | Backup content G1: `settings.json` (`/settings`), `actions.json` (`/settings/actions`) | `AbstractG1Device.backup` | `/settings`, `/settings/actions` | G1 | Same ZIP layout where it costs nothing; compatibility with ShellyScanner not required (Q11) | 6 | ✅ |
| B3 | Backup content G2+: `Shelly.GetDeviceInfo.json`, `Shelly.GetConfig.json`, `Schedule.List.json`, `Webhook.List.json`, `KVS.GetMany.json` (paged, merged), `Script.List.json`, `Shelly.GetComponents.json` (dynamic only, paged), `SensorAddon` section if add-on, one `<script name>.mjs` per script, plus model-specific entries | `AbstractG2Device.backup`, per-model `backup(ZipOutputStream)` | the listed RPC methods + `Script.GetCode` | G2+ | Same | 6 | ✅ |
| B4 | Backup battery G2+: DeviceInfo, Config, Webhook.List, KVS; falls back to stored JSON when asleep | `AbstractBatteryG2Device.backup` | same | G2+ battery | Same | 6 | ✅ |
| B5 | Backup BLU (BTHome: parent components + webhooks; TRV: remote device info/config…) | `blu/BTHomeDevice.backup`, `blu/BluTRV.backup` | `Shelly.GetComponents`, `Webhook.List`, `BluTrv.GetRemote*` | BLU | Same | 6 | ✅ |
| B6 | Restore single device: model compatibility check, "restore host X onto Y?" question, warnings (add-on, XMOD IO, BTHome, LoRa), errors (model, mode cover/thermostat/triphase, profile, power base), asks passwords not in backup (login, Wi-Fi 1/2/AP, MQTT), script override/enable/skip, then offers reboot | `controller/RestoreAction`, `RestoreMsg`, `restoreCheck` + `restore` per class, `RestoreUtil.compatibleModels` | many (see S13) | all | Restore wizard modal | 6 | ✅ |
| B7 | Restore G1 order: model-specific → cloud → `/settings` common (name, discoverable, timezone, lat/lng, tz*, allow_cross_origin, pon_wifi_reset, coiot, sntp) → login → MQTT → actions → roaming → Wi-Fi 2 → Wi-Fi 1 last | `AbstractG1Device.restore`, `restoreCommons`, `Actions.restore` | `/settings…`, `/settings/actions?index=…`, `/settings/login`, `/settings/mqtt`, `/settings/sta(1)` | G1 | Same order | 6 | ✅ |
| B8 | Restore G2+ order: model-specific → dynamic components → BLE, Cloud, Sys (device, sntp, debug), Matter, Zigbee, MQTT → schedules → scripts → KVS → webhooks → Wi-Fi 2, Wi-Fi 1, AP + roaming → login last | `AbstractG2Device.restore`, `restoreCommonConfig`, `DynamicComponents.restore`, `ScheduleManager.restore`, `Script.restoreAll`, `KVS.restoreKVS`, `Webhooks.restore`, `WIFIManagerG2.restore*` | `BLE/Cloud/Sys/Matter/Zigbee/MQTT.SetConfig`, `Schedule.*`, `Script.*`, `KVS.Set`, `Webhook.*`, `WiFi.SetConfig`, `Shelly.SetAuth`, … | G2+ | Same order | 6 | ✅ |
| B9 | Multi-device restore from a folder (file per hostname): no login/Wi-Fi/AP/MQTT passwords, scripts overridden, no reboot offer | `RestoreAction` (multi), `RestoreAction.nonInteractiveRestoreDevice` | as B6 | all | Newest stored backup of each device instead of a folder (P6-6, accepted); API variant `restore/multi` | 6 | ✅ |
| B10 | Restore of offline/ghost device queued as deferred | `RestoreAction` | — | all | Same | 6 | ✅ |
| B11 | Restore scripts from a backup file into the scripts panel | `ScriptsPanel` (upload from .sbk) | `Script.Create/PutCode` | G2+ | Scripts modal | 8 | ✅ |

### 1.9 Charts

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| G1 | Live time-series charts for selected devices; types: internal temp, RSSI, P, P sum, Q, S, V, VL, VX (custom expression), I, T all, H, HD, lux, frequency, distance, EM | `view/chart/MeasuresChart`, `ChartType` | samples from the refresh loop | all | Charts page (Chart.js) | 8 | ✅ |
| G2 | Range (auto / fixed windows), series selection, pause, markers, zoom (drag/wheel), scroll, Ctrl+P pause, help link | `MeasuresChart` | — | — | Same controls | 8 | ✅ |
| G3 | Export series CSV (horizontal / vertical, separator, current range when paused) | `view/chart/TimeChartsExporter` | — | — | Download CSV | 8 | ✅ |
| G4 | Default chart type setting; only pertinent types offered | settings `CHART_DEF`, `CHART_EXPORT` | — | — | Same | 8 | ✅ |
| G5 | Samples exist only while the chart window is open | `MeasuresChart` (model listener) | — | — | **Server-side in-memory ring buffer** so charts open with history (Q7); slower sampling while no browser is connected (Q8) | 8 | ✅ |

### 1.10 Application settings, archive, misc

| # | Feature | Where in Java | Shelly API | Gen | Web equivalent | Phase | Status |
|---|---|---|---|---|---|---|---|
| A1 | General: toolbar captions, font size (small/normal/big), default filter column, uptime format, temperature unit, double-click action, update check (never/stable/all), detailed-view resize, columns (default/detailed), CSV separator, default chart | `view/appsettings/PanelGUI`, `ScannerProperties` | — | — | Settings page (General) + per-browser Appearance (theme, palette, contrast, font, size — from MikroDash). Phase 1: language + appearance done; ShellyScanner options follow with their features — captions not needed (T17), update check with A8 | 1/3 | ✅ ⚠️ |
| A2 | Network: scan mode, IP ranges, status refresh interval, config refresh tics, restricted-login credentials (with "not secured" warning) | `view/appsettings/PanelNetwork` | — | — | Settings page (Network); credentials encrypted ⚠️ | 2 | ✅ |
| A3 | Archive: use archive, file, auto-reload, clear (confirm) | `view/appsettings/PanelStore`, `DevicesStore` | — | — | Settings page (Archive); file is fixed `/data/archive.json`; **no `.arc` import** (Q11) | 2 | ✅ ⚠️ |
| A4 | Script editor settings | `view/appsettings/PanelIDE` | — | — | Settings (Script editor), per browser | 8 | ✅ |
| A5 | Archive written on exit: identity, address, type, SSID, last connection, battery flag, notes, keyword; devices with errors/not logged keep stored data | `DevicesStore.store/read/toGhosts/getGhost` | — | all | Archive service; written on change + periodically (server has no "exit") ⚠️ | 2 | ✅ |
| A6 | Remove ghost devices from archive (warning if notes) | `MainView.eraseGhostAction` | — | — | Row action + confirm | 2 | ✅ |
| A7 | About dialog (author, licence, links) | `view/DialogAbout`, `aboutApp` label | — | — | About page: credits usnasoft + link, GPL-3.0, independent-project and trademark notice (EN/NL) | 1 | ✅ |
| A8 | Application update check against usna.it; skip release | `view/util/ApplicationUpdateCHK` | `https://www.usna.it/shellyscanner/last_version.txt` | — | **GitHub releases of ShellyLanMan, opt-in, off by default** (Q18) | 10 | ✅ ⚠️ |
| A9 | Localisation: English and Italian label bundles | `resources/LabelsBundle*.properties` | — | — | **English + Dutch** string catalogues (Q21); Italian not planned | 3 | ⚠️ |
| A10 | Online help links (manual, checklist, charts) | `UsnaOpenUrlAction`, `*ManualUrl` labels | — | — | **Own short in-app help** (Q22); usna.it linked from About | 8 | ✅ |
| A11 | Keyboard shortcuts (filter, tabs, pause, macOS cmd-C/V/X) | various | — | — | Web equivalents where sensible | 8 | ✅ |

### 1.11 Command line

| # | Feature | Where in Java | Web equivalent | Phase | Status |
|---|---|---|---|---|---|
| L1 | `-fullscan`/`-full`, `-localscan`/`-local`, `-ipscan a.b.c.x-y` (+ `-ipscan1..`), `-noscan` | `Main`, `CLIController.getIPCollection` | Settings (scan mode); no CLI (Q14) | 2 | ⚠️ |
| L2 | `-backup <dir>` non-interactive backup of all (or filtered) devices, prints result per host | `CLIController.backup` | API `POST /api/v1/backup` only; no CLI (Q14) | 6 | ⚠️ |
| L3 | `-restore <dir>` non-interactive restore (files per hostname) | `CLIController.restore`, `RestoreAction.nonInteractiveRestoreDevice` | same — API `POST /api/v1/restore/multi` (newest backups, P6-6) | 6 | ⚠️ |
| L4 | `-list [ip|full]` | `CLIController.list` | API `GET /api/v1/devices` only (Q14) | 2 | ⚠️ |
| L5 | `-gen <n>` filter for the above | `CLIController.getFilter` | query parameter — API `GET /api/v1/devices?gen=` | 2 | ✅ |
| L6 | `-graphs <TYPE>` headless stream `graph_data->host:channel:type:time:value` to stdout | `NonInteractiveMeasuresChart`, `MeasuresChart.setDoOutStream` | API/WebSocket stream (Q14) | 8 | ✅ |
| L7 | `-font <multiplier>` | `Main` | per-browser font size | 1 | ⚠️ |
| L8 | `-slow <ms>` extra delay for MQTT settings | `Main` → `MQTT_SLOW`, `PanelMQTT*` | setting (advanced) | 5 | ✅ |

---

## 2. Device types in ShellyScanner 1.3.4

Identified in `model/DevicesFactory.java`. Type names as shown in the "Type" column. ID = `type` (G1),
`app` (G2/G3; `/` separates alternative IDs such as `…ProAddon` and the `model` string used in code),
`model` (G4).

**Gen1 (27 + generic):** Button 1 (SHBTN-2), Shelly 1 (SHSW-1), Shelly 1L (SHSW-L), Shelly 1PM (SHSW-PM),
Shelly 2 (SHSW-21), Shelly 2.5 (SHSW-25), Shelly 3EM (SHEM-3), Shelly Bulb (SHBLB-1), DUO (SHBDUO-1),
DUO RGB (SHCB-1), Shelly DW (SHDW-1), Shelly DW2 (SHDW-2), Shelly Dimmer (SHDM-1), Shelly Dimmer 2 (SHDM-2),
Shelly EM (SHEM), Shelly Flood (SHWT-1), Shelly H&T (SHHT-1), Shelly I3 (SHIX3-1), Motion 1 (SHMOS-01),
Motion (SHMOS-02), Plug (SHPLG-1), Plug E (SHPLG2-1), PlugS (SHPLG-S), Plug US (SHPLG-U1),
Shelly RGBW2 (SHRGBW2), TRV (SHTRV-01), Shelly UNI (SHUNI-1); Generic G1.

**Gen2 (33 + generic):** Shelly BLU Gateway (BluGw), Shelly Mini 1 (Plus1Mini), Mini 1PM (Plus1PMMini),
Mini PM (PlusPMMini), +Dimmer 0-10V (Plus10V), +1 (Plus1), +1PM (Plus1PM), +2PM (Plus2PM), +H&T (PlusHT),
Plug +IT (PlusPlugIT), Plug +S (PlusPlugS), Plug +UK (PlusPlugUK), Plug +US (PlugUS), +RGBW (PlusRGBWPM),
Smoke (PlusSmoke), +UNI (PlusUni), +i4 (PlusI4), Pro 1 (Pro1, Pro1ProAddon), Pro 1PM, Pro 2, Pro 2PM
(each also …ProAddon), Pro 2CB (ProCB — "based on an obsolete prototype"), Pro 3 (Pro3), Pro 3EM
(Pro3EM, Pro3EMProAddon), Pro 4PM (Pro4PM), Pro Dual Cover (Pro4PM + model SPSH-002PE16EU),
Pro Dimmer 1PM / 2PM (ProDimmerx, distinguished by model), Pro EM-50 (ProEM), Pro RGBWW PM (ProRGBWWPM),
Wall Dimmer (PlusWallDimmer), Wall Display (WallDisplay), Wall Display X2i (WallDisplayV2); Generic G2.

**Gen3 (28 + generic):** Ogemray SW40 (Ogemray25), LinkedGo ST1820 and ST802 (XT1 + `svc0.type`),
generic XT1, Dimmer 0/1-10V G3, 1 G3, 1L G3, 1PM G3, 2L G3, 2PM G3, 3EM-63 (S3EMG3), Duo bulb G3,
RGB bulb G3 (RGBCCTBulbG3), Camera, Dimmer G3, EM G3, BLU Gateway G3, H&T G3, i4 G3, Mini 1 G3,
Mini 1PM G3, Mini PM G3, Plug M G3, Plug PM G3, Plug S G3, Outdoor Plug S G3, Shutter G3
(S2PMG3Shutter), X MOD1; Generic G3.

**Gen4 (15 + generic):** Dimmer 0/1-10V G4, 1 G4, 1L G4, 1PM G4, 2L G4, 2PM G4, Dimmer G4, EM G4,
Flood G4, Flood S G4, Mini 1 G4, Mini 1PM G4, Mini EM G4, Power Strip G4, Presence G4; Generic G4.
(13 mains-powered + 2 battery models, matched on the `model` string.)

**BLU:** BTHome devices (model from `attrs.model_id`: BLU Button, H&T, Door/Window, Motion, Remote,
Distance, RC Button 4, …), BLU TRV; Unmanaged BTHome.

Each of these gets a registry entry and at least one recorded fixture (`testdata/`). Models I own no
fixture for are listed per phase and asked for (see `DECISIONS.md` §5).

---

## 3. Shelly API surface used by ShellyScanner

Collected by grepping all `getJSON`, `sendCommand`, `postCommand`, `/rpc/…` calls in the source.

**Gen1 HTTP endpoints:** `/shelly`, `/settings` (+ many query parameters), `/status`, `/settings/actions`,
`/settings/cloud`, `/settings/login`, `/settings/mqtt`, `/settings/sta`, `/settings/sta1`, `/settings/ap`,
`/settings/relay/<n>`, `/settings/roller/<n>`, `/settings/light/<n>`, `/settings/color/<n>`,
`/settings/input/<n>`, `/settings/emeters/<n>`, `/settings/power/0`, `/settings/ext_temperature/<n>`,
`/settings/ext_humidity/0`, `/settings/ext_switch/0`, `/settings/adc/0`, `/settings/thermostats/<n>`,
`/settings/night_mode`, `/settings/warm_up`, `/relay/<n>`, `/roller/<n>`, `/light/<n>`, `/color/<n>`,
`/white/<n>`, `/ota`, `/ota/check`, `/reboot`, `/debug/log`, `/debug/log1`, `/cit/d` (CoIoT description,
info only), `/status/value`.

**Gen2+ RPC methods (with call counts in the source):**
AddOn.GetInfo · BLE.CloudRelay.ListInfos · BLE.SetConfig · BTHome.AddSensor · BTHome.DeleteSensor ·
BTHome.GetConfig · BTHome.GetStatus · BTHomeDevice.GetConfig · BTHomeDevice.GetKnownObjects ·
BTHomeDevice.GetStatus · BTHomeDevice.SetConfig · BTHomeSensor.GetConfig/GetStatus · BluGw.SetConfig ·
BluTrv.Call · BluTrv.CheckForUpdates · BluTrv.GetConfig · BluTrv.GetRemoteConfig ·
BluTrv.GetRemoteDeviceInfo · BluTrv.GetRemoteStatus · BluTrv.GetStatus · BluTrv.SetConfig ·
BluTrv.UpdateFirmware · Boolean.Set · CB.GetLog · CB.Set · CB.SetConfig · CCT.Set · CCT.SetConfig ·
Camera.AddZone · Camera.DeleteZone · Camera.Set · Camera.SetConfig · CameraZone.SetConfig ·
Cloud.SetConfig · Cover.SetConfig (+ Cover.Open/Close/Stop/GoToPosition) · EM.SetConfig · EM1.SetConfig ·
EM1Data.GetData · EM1Data.GetNetEnergies · EMData.GetData · EMData.GetRecords · Eth.SetConfig ·
Flood.SetConfig · Group.Set · HT_UI.SetConfig · Humidity.SetConfig · Illuminance.SetConfig ·
Input.SetConfig · Input.Trigger · KNX.GetConfig · KVS.Delete · KVS.GetMany · KVS.Set · LNM.Create ·
LNM.Delete · LNM.SetConfig · Light.Set · Light.SetConfig · Light.Toggle · LoRa.SetConfig ·
MQTT.GetConfig · MQTT.SetConfig · Matter.GetConfig · Matter.GetSetupCode · Matter.GetStatus ·
Matter.SetConfig · Number.Set · PLUGPM_UI / PLUGS_UI / PLUGUK_UI / POWERSTRIP_UI / WD_UI / Ui .SetConfig ·
PM1.SetConfig · PlusRGBWPM.SetConfig · ProRGBWWPM.SetConfig · Presence.AddZone · Presence.DeleteZone ·
Presence.SetConfig · PresenceZone.GetStatus · PresenceZone.SetConfig · RGB.Set · RGB.SetConfig ·
RGBCCT.Set · RGBCCT.SetConfig · RGBW.Set · RGBW.SetConfig · Schedule.Create · Schedule.Delete ·
Schedule.DeleteAll · Schedule.List · Schedule.Update · Script.Create · Script.Delete · Script.GetCode ·
Script.GetConfig · Script.List · Script.PutCode · Script.SetConfig · Script.Start · Script.Stop ·
SensorAddon.AddPeripheral · SensorAddon.GetPeripherals · Service.GetConfig · Service.SetConfig ·
Shelly.CheckForUpdate · Shelly.DetectLocation · Shelly.GetComponents · Shelly.GetConfig ·
Shelly.GetDeviceInfo · Shelly.GetStatus · Shelly.Reboot · Shelly.SetAuth · Shelly.SetProfile ·
Shelly.Update · Smoke.SetConfig · Storage.SetConfig · Switch.Set · Switch.SetConfig · Switch.Toggle ·
Sys.GetStatus · Sys.SetConfig · Temperature.SetConfig · Thermostat.GetStatus · Thermostat.Schedule.* ·
Thermostat.SetConfig · Virtual.Add · Virtual.Delete · Voltmeter.SetConfig · Webhook.Create ·
Webhook.Delete · Webhook.DeleteAll · Webhook.List · WiFi.ListAPClients · WiFi.SetConfig ·
Wifi.GetConfig · Wifi.SetConfig · XMOD.GetInfo · XMOD.GetProductJWS · Zigbee.SetConfig.

**WebSocket:** `ws://host/rpc` (events: `NotifyEvent` with `ota_progress`, `ota_success`,
`scheduled_restart`), `ws://host/debug/log` (Gen2+ logs).

**mDNS:** browse `_http._tcp.local.` (the `_shelly._tcp.local.` listener exists but is commented out).

**Internet:** `https://www.usna.it/shellyscanner/last_version.txt` (app update check only).

A per-generation recording of each of these, request and response, is what `testdata/` will hold.

---

## 4. Features that cannot transfer directly to a web application

### 4.1 Desktop-only behaviour

| Feature | How Java does it | Web approach | Browser limitation |
|---|---|---|---|
| Blocking credentials prompt during discovery | `DevicesFactory` opens a modal `DialogAuthentication` from a worker thread and waits (synchronized) until the user answers; then reuses the credentials for the next protected device | Discovery never blocks: protected devices appear with status "not logged" and a "Login" action; a banner offers to enter credentials once and apply them to all not-logged devices; stored credentials (encrypted) are tried first | A server cannot wait for a browser that may not be open |
| File choosers for backup/restore/scripts/CSV | `JFileChooser`, last path remembered | Backups written to `/data/backups` and downloadable; restore by picking a stored backup **or** uploading a `.sbk`; CSV and scripts as downloads/uploads | No access to the user's file system except via upload/download |
| Window geometry, detailed-view resize modes, toolbar captions, Nimbus font multiplier | `MainWindow.storeProperties`, `DETAIL_SCREEN`, `T_CAPTIONS`, `FONT_SIZE`, `-font` | Responsive layout; per-browser appearance settings (font, size, palette); detailed view = wider column set with horizontal scroll | A page cannot resize the browser window |
| Open device Web UI with `Desktop.browse` | `MainView.browseAction` | `<a target="_blank" href="http://ip:port">` | The **browser's** machine must be able to reach the device (fine on the LAN; not through a remote reverse proxy) |
| Print | `JTable.print` | Print stylesheet + `window.print()` | Layout is the browser's |
| Multiple independent windows (charts, info, logs, checklist, scripts IDE open side by side) | Non-modal `JDialog`/`JFrame`s | Pages and side panels; "open in new tab" links for charts/logs/device info | — |
| Settings in the user's home directory | `~/.shellyScanner`, `~/ShellyStore.arc` | `/data/settings.json`, `/data/archive.json`; no import from ShellyScanner (Q11) | — |
| Archive written on exit | `MainView.storeProperties` on window close | Written on every change (debounced) and on shutdown | A server has no "exit" |
| Per-user state vs shared state | Single user per process | Shared state (devices, archive, notes, deferred) is server-side; view preferences (columns, filters, appearance) per browser | Several people may use it at once — last write wins for notes |

### 4.2 Network behaviour inside a container

| Feature | How Java does it | Web approach | Limitation |
|---|---|---|---|
| mDNS full/local scan | JmDNS on the host's interfaces | Host networking (default) | Not available in bridge networking; UI banner + IP scan (ARCHITECTURE §2.5) |
| `isReachable` pre-ping in IP scan | `InetAddress.isReachable(10 s)` (ICMP as root, TCP port 7 otherwise) | Skip; connect to port 80 with a short timeout | Deliberate difference, no functional loss (the `/shelly` probe decides anyway) |
| Update check to usna.it | direct HTTPS | GitHub releases of ShellyLanMan, opt-in (A8, Q18) | Outbound internet, off by default |

### 4.3 Things that are simply different on the web

- **Live updates**: the Swing table repaints on model events; the web table receives the same events
  over WebSocket and patches rows.
- **Charts**: JFreeChart → Chart.js; behaviour kept (types, range, pause, zoom, export).
- **Script IDE**: Swing text editor with custom features → an embedded code editor (proposal:
  CodeMirror 6, MIT, loaded only on the Scripts page; see `DECISIONS.md` §1.4).

---

## 5. Observations in the original (to confirm with the author, not silently "fixed")

| # | Observation | Where | Proposed handling |
|---|---|---|---|
| O1 | `restoreCommonConfig` MQTT condition: `mqtt.isMissingNode() == false && userPref.containsKey(...) || … || …` — operator precedence means the "missing node" guard only applies to the first term | `g2/AbstractG2Device.restoreCommonConfig` | Keep behaviour; analyse further in Phase 6 (author not contacted, Q23) |
| O2 | Battery G2 FW fallback sets `stableBuild = current = …available stable version` — "current" is overwritten with the available version when only stored `GetStatus` exists | `g2/modules/FirmwareManagerG2.init` (catch branch) | Analyse and test on a battery device in Phase 7; likely should not set `current` there |
| O3 | Gen1 restore list contains `tz_dst_auto` twice; G2 restore sets `sntp` twice | `AbstractG1Device.restoreCommons`, `AbstractG2Device.restoreCommonConfig` | Harmless; replicate the effective behaviour once |
| O4 | Method names `Wifi.GetConfig`/`Wifi.SetConfig` (lower-case "fi") next to `WiFi.SetConfig` | `g2/modules/WIFIManagerG2` | Works on devices (RPC names are case-insensitive — to verify); we use `WiFi.*` |
| O5 | Input "execute" makes the **scanner** call the URLs configured in G1 actions / G2+ webhooks (with `127.0.0.1` rewritten to the device) | `g1/modules/Actions.execute`, `g2/modules/Webhooks.execute` | Keep; the server will issue those GETs. Only URLs read from the device, only on explicit user action |
| O6 | Discovery credentials are one global "last used" user/password; a device prompted with a new password changes the default for the following ones | `DevicesFactory.setCredential`, `digestAuthentication` | Decided: per-device credentials with a global default (Q9) |
| O7 | IP-scan `/shelly` timeout is 80 s | `Devices.isShelly` | Use a shorter timeout; verify on hardware that no device needs the long one |
| O8 | CLI `-restore` without path prints "mandatory parameter after -backup" | `CLIController.restore` | Cosmetic |
| O9 | `ShellyPro2CB` "do not include; based on an obsolete prototype" but still in the factory | `DevicesFactory.createG2` | Include for parity |
| O10 | `BLEDevice`, `BLEGateway` are unused drafts (commented-out code in `Devices.create`) | `blu/BLEDevice`, `blu/BLEGateway` | Not ported |
| O11 | The Gen2 **BLU Gateway** (`ShellyGateway`, app `BluGw`) is not scanned for BTHome devices: `Devices.create` only scans `AbstractProDevice`, Gen3 and Gen4 | `model/Devices.java` | Replicated; to verify on a Gen2 BLU Gateway if someone has one |
| O12 | Archive "Auto reload" only runs in the mDNS scan modes (not after an IP scan) | `Devices.scannerInit`, `PanelStore` tooltip | Replicated |
| O13 | `ShellyPro4PM.fillStatus` reads the input state of channels 3 and 4 from `input:1` instead of `input:2` / `input:3` | `g2/ShellyPro4PM.java` | Replicated (only the input indicator); probably a typo — confirm on a Pro 4PM |
| O14 | `LightBulbRGB.change(on)` (Gen1 Bulb / DUO RGBW) sends `/light/?turn=on` without the channel index; `toggle()` uses `/light/0` | `g1/modules/LightBulbRGB.java` | We send `/light/0?turn=…` (switch-all in the lights editor) — accepted by Wim 2026-09-27 (unknown whether the index-less form works) |
| O15 | `ThermostatG2.setTargetTemp/setEnabled` post `{"config":{"target_C"=20}}` — `=` instead of `:` | `g2/modules/ThermostatG2.java` | We send valid JSON (`"target_C":20`) — accepted by Wim 2026-09-27 |
| O16 | BLU TRV reboot uses `GET /rpc/BluTrv.call?id=N&method=Shelly.Reboot` (lower-case `call`, method not quoted) while the other TRV calls use `BluTrv.Call` with a JSON method string | `blu/BluTRV.reboot` | Replicated as-is; verify on a TRV |
| O17 | `ShellyPro2PM` in relay mode with a Pro add-on but no digital output adds `null` to its relays (`new Relay[] {relay0, relay1, sensorAddOn.getDigitalOut()}`) | `g2/ShellyPro2PM.java` | We add the digital output only when it exists |
| O18 | `DevicesCommandCellEditor` shows a thermostat only when it is the only kind of module; the Wall Display therefore shows **either** its thermostat (when `thermostat:0` is configured) **or** its relay, never both | `g2/WallDisplay.getModules` | Replicated |
| O19 | `WIFIManagerG2.disable` posts `{"config": {sta:{"enable": false}}}` — the key `sta` is not quoted | `g2/modules/WIFIManagerG2.java` | We send valid JSON |
| O20 | `InputResetManagerG2.enableReset` posts `{"id":N,"config":{"factory_reset":true}` — one closing brace missing | `g2/modules/InputResetManagerG2.java` | We send valid JSON |
| O21 | `InputResetManagerG2`: when the inputs disagree ("MIX") the loop stops, so a later enable/disable only changes the inputs read before the first disagreeing one | `g2/modules/InputResetManagerG2.java` | We change every input that has `factory_reset` — accepted by Wim 2026-09-27 |
| O22 | The deferred-task list is cancelled on every rescan (`DeferrablesContainer` on `CLEAR`), because it is keyed by table row | `controller/DeferrablesContainer.java` | Tasks are keyed by device ID and survive rescans and restarts (Q10) |
| O23 | Gen1 restore of external sensors appends the JSON node of `ext_sensors.temperature_unit` to the URL, quotes included (`…unit="C"`) | `g1/AbstractG1Device` / `ShellyUniG1`, `Shelly1`/`1PM` restore | We send the value without quotes — accepted by Wim 2026-09-27 |
| O24 | Shelly Flood restore: the second half of the `/settings` query (`rain_sensor`, `temperature_offset`) is concatenated outside the request call, so it is never sent | `g1/ShellyFlood.restore` | We send both halves in one request — accepted by Wim 2026-09-27 |
| O25 | Shelly Pro 2 CB restore posts the voltmeter configuration with `CB.SetConfig` instead of `Voltmeter.SetConfig` | `g2/ShellyPro2CB.restore` | We use `Voltmeter.SetConfig` — accepted by Wim 2026-09-27 |
| O26 | A Gen1 restore with `tzautodetect=true` also sends the stored `lat`/`lng`; the device then locates itself again and the stored coordinates are replaced (seen on the PlugS, Phase 6 hardware test) | `AbstractG1Device.restoreCommons` | Same as the original (firmware behaviour); noted only |
| O27 | Battery Gen2+ firmware from stored data: when only `Shelly.GetStatus` is stored, the current version is set to the available stable version (`stableBuild = current = …`), so a sleeping device without stored `/shelly` or config shows no update | `g2/modules/FirmwareManagerG2.init` | Same as the original (noted, not fixed) — accepted by Wim 2026-09-27 |
| O28 | FW Update panel: "Select stable / beta / Deselect all" act on the selected table rows when more than one is selected, otherwise on **all** rows including those hidden by the filter; the table can be sorted | `PanelFWUpdate`, `FWUpdateTable` | The buttons act on the rows shown (filter applied); no row selection or sorting in the web table — accepted by Wim 2026-09-27 |
| O29 | CSV export writes every value as it is, without quoting: a device name or measure containing the separator breaks the columns | `controller/ExportCSVAction`, `view/chart/TimeChartsExporter` | Values with the separator, a quote or a line break are quoted (RFC 4180) — accepted by Wim 2026-09-27 |
| O30 | Opening the script editor for a running script (and running one from it) switches the device's websocket debug log on when it is off (`setDebugMode(SOCKET)`), a configuration write; it is never switched off again | `view/scripts/ide/ScriptFrame.runningStatus/activateLogConnection` | Same as the original — accepted by Wim 2026-09-27 |
| O31 | BLU TRV scheduler calls use `Trv.UpdateScheduleRule`, `Trv.AddScheduleRule`, `Trv.RemoveScheduleRule` ("Trv") while listing and the restore use `TRV.*` | `blu/modules/ScheduleManagerTRV` | Same names as the original (RPC method names are case-insensitive on the devices — not verified: no BLU TRV here) |
| O32 | The chart's P (sum) type is offered only when one device has two power meters, and adds the total of an EM total meter instead of the phases | `MeasuresChart.typeComboContent`, `newValue` | Same as the original |
| O33 | A device that cannot be read when it is discovered (e.g. still starting) stays "unmanaged, error" until Reload or a rescan: the only automatic retry is 30 s after the scan starts, and mDNS reports an instance once | `Devices.errorsReconnect`, `MDNSListener.serviceResolved` | Extension agreed by Wim 2026-09-28: failed devices are retried every 2 minutes (after the first retry at 30 s) |

## 6. Deliberate deviations from Phase 12 (first use, `DECISIONS.md` §21)

From Phase 12 on, ShellyLanMan keeps ShellyScanner's features and Shelly API use, but
not its page logic where that came from the desktop client (P12-1). The rows marked
"P12 §6" behave differently on purpose.

| Row | Original | ShellyLanMan from Phase 12 | Decision |
|---|---|---|---|
| T7, S6, F1, G1 | Each dialog (main table, checklist, FW update, charts) has its own row selection | One selection shared by Devices, Checklist, Firmware and Charts; Checklist and Firmware show the selected devices with *Show all*, Charts the selected devices only; Checklist has checkboxes | P12-2 |
| S6 | Disabled action buttons say nothing | The tooltip says why the button is disabled | P12-3, P12-4 |
| T20 | Confirm only for more than 8 devices; always a new browser window | Confirm for more than one device; ↗ on the button; per-browser setting new tab / this tab | P12-11 |
| D11 | Rescan shows archived devices as ghosts at once | Known devices show *searching* and are probed at once; archived only after the search window | P12-7 |
| D12 | Refresh reads every device | Refresh reads the selected devices, or all when none is selected | P12-6 |
| F1 | The FW table appears when every device has answered | Rows appear at once and are filled in per device | P12-1 |
