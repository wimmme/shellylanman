// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// model/device/GhostDevice.restoreCheck (G1, G2, BLU, BLU TRV).

package sbk

// CheckStored is the check for an archived device, made from the file
// alone: model, host and the passwords to ask. Network settings are not
// asked ("Can't restore network values"). The restore itself is then queued.
func CheckStored(d *Device, f Files) Check {
	res := Check{}
	switch {
	case f["settings.json"].Exists():
		s := f["settings.json"]
		if s.Path("device", "type").Text() != d.TypeID {
			res.put(ErrModel, "")
			return res
		}
		if host := s.Path("device", "hostname").Text(); host != d.Hostname {
			res.put(PreRestoreHost, host)
		}
		if s.Path("login", "enabled").Bool() {
			res.put(AskLogin, s.Path("login", "username").Text())
		}
		if s.Path("mqtt", "enable").Bool() && s.Path("mqtt", "user").Text() != "" {
			res.put(AskMQTT, s.Path("mqtt", "user").Text())
		}
	case f["Shelly.GetRemoteDeviceInfo.json"].Exists():
		info := f["Shelly.GetRemoteDeviceInfo.json"].Get("device_info")
		if !info.Exists() || info.Get("app").Text() != d.TypeID {
			res.put(ErrModel, "")
			return res
		}
		if host := info.Get("id").Text(); host != d.Hostname {
			res.put(PreRestoreHost, host)
		}
	case f["Shelly.GetConfig.json"].Exists():
		devInfo, config := f["Shelly.GetDeviceInfo.json"], f["Shelly.GetConfig.json"]
		if devInfo.Get("app").Text() != d.TypeID {
			res.put(ErrModel, "")
			return res
		}
		if host := devInfo.Get("id").Text(); host != d.Hostname {
			res.put(PreRestoreHost, host)
		}
		if devInfo.Get("auth_en").Bool() {
			res.put(AskLogin, "admin")
		}
		if config.Path("mqtt", "enable").Bool() && config.Path("mqtt", "user").Text() != "" {
			res.put(AskMQTT, config.Path("mqtt", "user").Text())
		}
	case f[bluFile].Exists():
		info := f[bluFile]
		name := info.Get("type").Text()
		if name != d.TypeID {
			res.put(ErrModel, "")
			return res
		}
		for _, c := range f["Shelly.GetComponents.json"].Get("components").Items() {
			if c.Get("key").Text() == "bthomedevice:"+info.Get("index").Text() {
				if mac := c.Path("config", "addr").Text(); mac != d.MAC {
					res.put(PreRestoreHost, name+"-"+mac)
				}
				return res
			}
		}
		res.put(ErrModel, "")
	default:
		res.put(ErrModel, "")
	}
	return res
}
