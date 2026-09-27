# Provenance

Where each part of ShellyLanMan comes from, for honest attribution
(`DECISIONS.md` §3.2). Categories: **1** copied from ShellyScanner · **2** ported
(logic translated from the Java code — derivative work) · **3** clean
re-implementation from the Shelly API documentation and observed behaviour ·
**4** new. Files in category 2 carry a "Portions derived from ShellyScanner"
header. Update this table in the same commit as the code.

| Path | Category | Source / notes |
|---|---|---|
| `cmd/shellylanman` | 4 | — |
| `cmd/record` | 4 | request list mirrors ShellyScanner's info requests (`AbstractG1Device`/`AbstractG2Device.getInfoRequests`) — a list of public API calls, not code |
| `cmd/shellysim`, `internal/sim` | 4 | — |
| `internal/fixture` | 4 | — |
| `internal/model/registry.go` | 2 | `model/DevicesFactory.java`, type IDs and names of `model/device/g1..g4`, `blu/BTHomeDevice` |
| `internal/model/device.go` | 2 | status set (`ShellyAbstractDevice.Status`), MAC from host name (`ShellyGenericUnmanagedImpl`) |
| `internal/service/devices.go`, `archive.go` | 2 | `model/Devices.java` (discovery flow, replacement rules, refresh scheduling, retries), `model/DevicesStore.java` |
| `internal/service/blu.go` | 2 | `Devices.create/newBluDevice`, `blu/BTHomeDevice`, `blu/BluTRV`, `blu/modules/SensorsCollection` |
| `internal/service/credentials.go` | 4 | (ShellyScanner prompts; here stored per device / global) |
| `internal/shelly` | 3 | Shelly API docs (Gen1 REST, Gen2+ RPC, digest authentication); pacing value from `Devices.MULTI_QUERY_DELAY` |
| `internal/discovery/mdns.go` | 3 | RFC 6762/6763; service type from `Devices.SERVICE_TYPE1` |
| `internal/discovery/ipscan.go` | 2 | `IPCollection`, `Devices.scanByIP` |
| `internal/store/archive.go` | 2 | field names of `DevicesStore` |
| `internal/parse/gen1.go` | 2 | fillSettings/fillStatus and meters of `model/device/g1/*`, g1 modules |
| `internal/parse/gen2.go` | 2 | `AbstractG2Device`, fillStatus/meters of `model/device/g2`, `g3`, `g4` classes, `g2/modules/SensorAddOn(Pro)`, `g2/meters/*`, g2 modules |
| `internal/parse/blu.go` | 2 | `blu/modules/Sensor`, `SensorsCollection`, `blu/BluTRV` |
| `internal/service/info.go` | 2 | `getInfoRequests()` per class, `DialogDeviceLogsG1/G2`, `LoginManagerG2.getAuthString` |
| `internal/parse/gen2_ctl.go` | 2 | `g2/modules/SensorAddOnPro.getDigitalOut`, `g3/PbSXT1St1820`, `g3/PbSXT1St802`, `g3/modules/XT1Thermostat` |
| `internal/parse/actions.go` | 2 | `g1/modules/Actions` (fillSettings, Action.isActive) |
| `internal/ojson` | 4 | (ordered JSON model standing in for Jackson's `ObjectNode`) |
| `internal/sbk/backup.go` | 2 | `AbstractG1Device.backup`, `AbstractG2Device.backup`, `AbstractBatteryG2Device.backup`, backup of `ShellyPlusUNI`, `WallDisplay(X2i)`, `PbSXT1*`, `ShellyXMOD1`, `BTHomeDevice`, `BluTRV`; `RestoreAction.readBackupFile` |
| `internal/sbk/restore.go`, `device.go` | 2 | `RestoreMsg`, `AbstractG1Device.restoreCheck/restore/restoreCommonsG1`, `g1/modules/*` restore, `AbstractG2Device.restoreCheck/restore/restoreCommonConfig`, `g2/modules/Webhooks`, `KVS`, `Script(s)`, `LoginManagerG2`, `WIFIManagerG2`, `Devices`/`getJSON`/`postCommand` semantics |
| `internal/sbk/g1.go` | 2 | `restoreCheck`/`restore` of every `model/device/g1` class and their modules |
| `internal/sbk/g2.go` | 2 | `restoreCheck`/`restore` of every `model/device/g2`, `g3`, `g4` class, `g2/modules/SensorAddOn(Pro)`, `LoRaAddOn`, `ThermostatG2`, `g3/modules/XT1Thermostat`, `WallDisplay`, `DynamicComponents` |
| `internal/sbk/ghost.go` | 2 | `GhostDevice.restoreCheck` |
| `internal/sbk/blu.go` | 2 | `BTHomeDevice.restoreCheck/restore`, `BluTRV.restoreCheck/restore`, `blu/modules/ScheduleManagerTRV`, `SensorsCollection.deleteAll` |
| `internal/service/backup.go` | 2 | `controller/BackupAction`, `controller/RestoreAction` (single and multi restore, deferred tasks, reboot offer) |
| `internal/service/firmware.go` | 2 | `FirmwareManager.getShortVersion`, `FirmwareManagerG1/G2/TRV`, `PanelFWUpdate` (rows, apply, deferral, `FMUpdateListener`, back-on-line rule) |
| `internal/service/control.go` | 2 | commands of `g1/modules/*`, `g2/modules/*`, `g3/modules/*`, `blu/BluTRV`; `MainView.rebootAction`, `Devices.reboot`, `Webhooks.execute`, `Actions.execute` |
| `internal/service/config.go` | 2 | `view/devsettings/*` panels; `WIFIManager*`, `LoginManager*`, `MQTTManager*`, `TimeAndLocationManager*`, `InputResetManager*`, `setCloudEnabled` |
| `internal/service/deferred.go` | 2 | `controller/DeferrableTask`, `DeferrablesContainer` |
| `internal/service/checklist.go` | 2 | `view/checklist/CheckListView` (rows, actions, gateways), `setEcoMode`, `setLEDMode`, `setDebugMode`, `RangeExtenderManager.enable`, `ScheduleManager` (auto FW) |
| `web/src/panels/devsettings.ts` | 2 | `view/devsettings/*`, `view/DialogDeviceSelection` |
| `web/src/pages/checklist.ts`, `web/src/checklistlogic.ts` | 2 | `view/checklist/*` |
| `web/src/pages/deferred.ts` | 2 | `view/DialogDeferrables`, `MainView` deferred button |
| `internal/shelly/rpc.go` | 2 | `AbstractG2Device.executeRPC/postCommand`, `LoginManagerG2.getAuthNode` |
| `web/src/command.ts`, `web/src/commandlogic.ts` | 2 | `view/DevicesCommandCellEditor`, `DevicesCommandCellRenderer`, `view/util/ColorUtil`, event labels of `LabelsBundle.properties` |
| `web/src/panels/backup.ts`, `web/src/restorelogic.ts` | 2 | `controller/BackupAction`, `controller/RestoreAction.restoreDevice`, `view/DialogAuthentication`, `msgRestore*`/`errRestore*`/`lbl_*` texts of `LabelsBundle.properties` |
| `web/src/panels/firmware.ts`, `web/src/firmwarelogic.ts`, `web/src/pages/firmware.ts` | 2 | `view/devsettings/PanelFWUpdate`, `FWUpdateTable`, FW labels of `LabelsBundle.properties` |
| `web/src/panels/lights.ts` | 2 | `view/lightsEditor/*` |
| `web/src/format.ts` | 2 | `METER_LBL_*`, `METER_VAL_*`, uptime formats of `LabelsBundle.properties` |
| `internal/store` | 4 | — |
| `internal/hub` | 4 | — |
| `internal/httpapi` | 4 | — |
| `internal/version`, `internal/web` | 4 | — |
| `web/src/appearance.ts` | adapted from MikroDash (MIT) | factor tables, brightness formula, neutral-level rule |
| `web/public/app.css` (tokens block) | from MikroDash (MIT) | palettes and design tokens |
| `web/public/app.css` (components), other `web/src/*` | 4 | — |
| `web/public/fonts/*` | third party (OFL-1.1) | via MikroDash |
| `web/src/pages/devices.ts` column headings | terminology from ShellyScanner (`LabelsBundle.properties`) | short UI terms |

Nothing is copied verbatim; ported logic is listed above (since Phase 2).
