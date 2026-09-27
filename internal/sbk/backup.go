// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// AbstractG1Device.backup, AbstractG2Device.backup, AbstractBatteryG2Device.backup,
// the backup(ZipOutputStream) of ShellyPlusUNI, WallDisplay(X2i), PbSXT1*,
// ShellyXMOD1, BTHomeDevice.backup, BluTRV.backup and
// RestoreAction.readBackupFile.

package sbk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
)

// Files is a backup's content: entry name → JSON. A script "x.mjs" is kept
// as "x.mjs.json" = {"code": "..."} (readBackupFile).
type Files map[string]*ojson.Value

// ErrNotBackup: the data is not a readable backup.
var ErrNotBackup = errors.New("not a backup file")

// Read opens a .sbk.
func Read(data []byte) (Files, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrNotBackup
	}
	out := Files{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > 16<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 16<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		name := f.Name[strings.LastIndex(f.Name, "/")+1:]
		if strings.HasSuffix(name, ".json") {
			v, err := ojson.Parse(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			out[name] = v
		} else {
			out[name+".json"] = ojson.Obj("code", string(b))
		}
	}
	return out, nil
}

type zipWriter struct {
	buf bytes.Buffer
	zw  *zip.Writer
}

func newZip() *zipWriter {
	z := &zipWriter{}
	z.zw = zip.NewWriter(&z.buf)
	return z
}

func (z *zipWriter) add(name string, data []byte) error {
	w, err := z.zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (z *zipWriter) addJSON(name string, v *ojson.Value) error {
	return z.add(name, []byte(v.String()))
}

func (z *zipWriter) bytes() ([]byte, error) {
	if err := z.zw.Close(); err != nil {
		return nil, err
	}
	return z.buf.Bytes(), nil
}

// section: sectionToStream — GET path (paged over arrayKey when given) into name.
func (d *Device) section(ctx context.Context, z *zipWriter, path, arrayKey, name string) (*ojson.Value, error) {
	var v *ojson.Value
	var err error
	if arrayKey != "" {
		v, err = d.paged(ctx, path, arrayKey)
	} else {
		v, err = d.get(ctx, path)
	}
	if err != nil {
		return nil, err
	}
	return v, z.addJSON(name, v)
}

// FileName: BackupAction.defFileName — the host name with anything but
// letters, digits, '_' and '-' replaced by '_'.
func FileName(hostname string) string {
	return nonWord.ReplaceAllString(hostname, "_") + ".sbk"
}

var nonWord = regexp.MustCompile(`[^\w\-]+`)

// Backup reads the device into a .sbk. stored is true when a sleeping
// battery device was backed up from its stored answers.
func Backup(ctx context.Context, d *Device) (data []byte, stored bool, err error) {
	switch {
	case d.Gen == "1":
		data, err = backupG1(ctx, d)
	case d.Gen == "bth":
		data, err = backupBTHome(ctx, d)
	case d.Gen == "blu":
		data, err = backupTRV(ctx, d)
	case d.Battery:
		return backupBatteryG2(ctx, d)
	default:
		data, err = backupG2(ctx, d)
	}
	return data, false, err
}

func backupG1(ctx context.Context, d *Device) ([]byte, error) {
	z := newZip()
	if _, err := d.section(ctx, z, "/settings", "", "settings.json"); err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/settings/actions", "", "actions.json"); err != nil {
		return nil, err
	}
	return z.bytes()
}

func backupG2(ctx context.Context, d *Device) ([]byte, error) {
	z := newZip()
	if _, err := d.section(ctx, z, "/rpc/Shelly.GetDeviceInfo", "", "Shelly.GetDeviceInfo.json"); err != nil {
		return nil, err
	}
	config, err := d.section(ctx, z, "/rpc/Shelly.GetConfig", "", "Shelly.GetConfig.json")
	if err != nil {
		return nil, err
	}
	_, _ = d.section(ctx, z, "/rpc/Schedule.List", "", "Schedule.List.json") // unmanaged battery devices have none
	if _, err := d.section(ctx, z, "/rpc/Webhook.List", "", "Webhook.List.json"); err != nil {
		return nil, err
	}
	_, _ = d.section(ctx, z, "/rpc/KVS.GetMany", "items", "KVS.GetMany.json")                                          // not on every model
	scripts, _ := d.section(ctx, z, "/rpc/Script.List", "", "Script.List.json")                                        // not on every model
	_, _ = d.section(ctx, z, "/rpc/Shelly.GetComponents?dynamic_only=true", "components", "Shelly.GetComponents.json") // Pro, Gen3+
	if config.Path("sys", "device", "addon_type").Str("") == addonSensor {
		if _, err := d.section(ctx, z, "/rpc/SensorAddon.GetPeripherals", "", peripheralsFile); err != nil {
			return nil, err
		}
	}
	if scripts != nil {
		for _, s := range scripts.Get("scripts").Items() {
			code := ""
			if v, err := d.get(ctx, fmt.Sprintf("/rpc/Script.GetCode?id=%d", s.Get("id").Int())); err == nil {
				code = crlf.ReplaceAllString(v.Get("data").Text(), "\n")
			} else if d.offline {
				return nil, err
			}
			if err := z.add(s.Get("name").Text()+".mjs", []byte(code)); err != nil {
				return nil, err
			}
		}
	}
	if err := backupModel(ctx, d, z); err != nil && d.offline {
		return nil, err
	}
	return z.bytes()
}

var crlf = regexp.MustCompile(`\r+\n`)

// backupModel: the device-specific sections.
func backupModel(ctx context.Context, d *Device, z *zipWriter) error {
	switch d.TypeID {
	case "PlusUni":
		if _, err := d.section(ctx, z, "/rpc/SensorAddon.GetPeripherals", "", peripheralsFile); err != nil {
			return err
		}
	case "WallDisplay", "WallDisplayV2":
		cfg, err := d.get(ctx, "/rpc/Shelly.GetConfig")
		if err != nil {
			return err
		}
		if cfg.Get("thermostat:0").Exists() {
			profiles, err := d.section(ctx, z, "/rpc/Thermostat.Schedule.ListProfiles?id=0", "", "Thermostat.Schedule.ListProfiles.json")
			if err != nil {
				return err
			}
			for _, p := range profiles.Get("profiles").Items() {
				id := p.Get("id").Text()
				if _, err := d.section(ctx, z, "/rpc/Thermostat.Schedule.ListRules?id=0&profile_id="+id, "", "Thermostat.Schedule.ListRules_profile_id-"+id+".json"); err != nil {
					return err
				}
			}
		}
	case "XT1":
		if d.Variant == xt1ST1820 || d.Variant == xt1ST802 {
			if _, err := d.section(ctx, z, "/rpc/Service.GetConfig?id=0", "", "Service.GetConfig.json"); err != nil {
				return err
			}
		}
	case "XMOD1":
		if _, err := d.section(ctx, z, "/rpc/XMOD.GetInfo", "", "XMOD.GetInfo.json"); err != nil {
			return err
		}
	}
	return nil
}

// backupBatteryG2: DeviceInfo, Config, Webhook.List and KVS; when the device
// sleeps, the stored answers of all four (AbstractBatteryG2Device.backup).
func backupBatteryG2(ctx context.Context, d *Device) ([]byte, bool, error) {
	z := newZip()
	err := func() error {
		for _, s := range [][3]string{
			{"/rpc/Shelly.GetDeviceInfo", "", "Shelly.GetDeviceInfo.json"},
			{"/rpc/Shelly.GetConfig", "", "Shelly.GetConfig.json"},
			{"/rpc/Webhook.List", "", "Webhook.List.json"},
		} {
			if _, err := d.section(ctx, z, s[0], s[1], s[2]); err != nil {
				return err
			}
		}
		_, _ = d.section(ctx, z, "/rpc/KVS.GetMany", "items", "KVS.GetMany.json")
		return nil
	}()
	if err == nil {
		b, err := z.bytes()
		return b, false, err
	}
	paths := []string{"/rpc/Shelly.GetDeviceInfo", "/rpc/Shelly.GetConfig", "/rpc/Webhook.List", "/rpc/KVS.GetMany"}
	names := []string{"Shelly.GetDeviceInfo.json", "Shelly.GetConfig.json", "Webhook.List.json", "KVS.GetMany.json"}
	for _, p := range paths {
		if d.Stored[p] == nil {
			return nil, false, err
		}
	}
	z = newZip()
	for i, p := range paths {
		if e := z.add(names[i], d.Stored[p]); e != nil {
			return nil, false, e
		}
	}
	b, e := z.bytes()
	return b, true, e
}

// bluFile holds the identity of a BTHome device in its backup.
const bluFile = "ShellyScannerBLU.json"

// backupBTHome: identity + the gateway's dynamic components and webhooks.
func backupBTHome(ctx context.Context, d *Device) ([]byte, error) {
	z := newZip()
	if err := z.addJSON(bluFile, ojson.Obj("index", d.BLUIndex, "type", d.TypeID, "mac", d.MAC)); err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/rpc/Shelly.GetComponents?dynamic_only=true", "components", "Shelly.GetComponents.json"); err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/rpc/Webhook.List", "", "Webhook.List.json"); err != nil {
		return nil, err
	}
	return z.bytes()
}

func backupTRV(ctx context.Context, d *Device) ([]byte, error) {
	z := newZip()
	i := d.BLUIndex
	if _, err := d.section(ctx, z, "/rpc/BluTrv.GetRemoteDeviceInfo?id="+i, "", "Shelly.GetRemoteDeviceInfo.json"); err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/rpc/BluTrv.GetRemoteConfig?id="+i, "", "Shelly.GetRemoteConfig.json"); err != nil {
		return nil, err
	}
	config, err := d.section(ctx, z, "/rpc/BluTrv.GetConfig?id="+i, "", "Shelly.GetConfig.json")
	if err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/rpc/BluTrv.Call?id="+i+"&method=%22TRV.ListScheduleRules%22&params=%7B%22id%22:0%7D", "", "TRV.ListScheduleRules.json"); err != nil {
		return nil, err
	}
	if _, err := d.section(ctx, z, "/rpc/Webhook.List", "", "Webhook.List.json"); err != nil {
		return nil, err
	}
	bthome := config.Get("trv").Text()
	if _, err := d.section(ctx, z, "/rpc/BTHomeDevice.GetKnownObjects?id="+bthome[strings.Index(bthome, ":")+1:], "", "BTHomeDevice.GetKnownObjects.json"); err != nil {
		return nil, err
	}
	return z.bytes()
}
