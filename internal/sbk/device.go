// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// AbstractG1Device.sendCommand, AbstractG2Device.postCommand/getJSON/
// getPagedJson and the Network detection of WIFIManagerG1/G2.currentConnection.

// Package sbk makes and restores ShellyScanner-style device backups (.sbk:
// a ZIP of JSON sections, plus one .mjs file per script). The request
// sequences follow the Java classes one by one; see restore*.go.
package sbk

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Device is what a backup or restore needs to know about the device.
type Device struct {
	Conn     *shelly.Conn
	Gen      string // "1".."4", "blu" (TRV), "bth" (BTHome)
	TypeID   string // registry type (parse package keys)
	App      string // Gen2+ "app" (ShellyScanner's type ID)
	Model    string // Gen2+ model code
	Variant  string // XT1 svc0.type
	Hostname string
	MAC      string // upper case, no separators
	SSID     string // network the device is connected to now
	Battery  bool
	Pro      bool

	// Stored answers of a sleeping battery device (path → body).
	Stored map[string][]byte

	// BLU devices: the component index on the gateway and, for BTHome
	// devices, the ids of their sensor components.
	BLUIndex   string
	BLUSensors []int

	offline         bool // a request failed because the device could not be reached
	restartRequired bool // a Gen2+ answer said restart_required
}

// RestartRequired: a restored setting needs a reboot (the original then offers one).
func (d *Device) RestartRequired() bool { return d.restartRequired }

// Offline reports whether a request of the last operation found the device off line.
func (d *Device) Offline() bool { return d.offline }

func (d *Device) note(err error) error {
	if shelly.IsOffline(err) {
		d.offline = true
	}
	return err
}

// get: GET path, parsed.
func (d *Device) get(ctx context.Context, path string) (*ojson.Value, error) {
	b, err := d.Conn.Get(ctx, path)
	if err != nil {
		return nil, d.note(err)
	}
	return ojson.Parse(b)
}

// cmd is AbstractG1Device.sendCommand: nil on success, else the message.
func (d *Device) cmd(ctx context.Context, path string) *string {
	_, err := d.Conn.Get(ctx, path)
	return msgPtr(d.note(err))
}

// post is AbstractG2Device.postCommand: nil on success, else the message.
func (d *Device) post(ctx context.Context, method string, params *ojson.Value) *string {
	res, err := d.call(ctx, method, params)
	if err == nil && res.Path("restart_required").Bool() {
		d.restartRequired = true
	}
	return msgPtr(err)
}

// call is AbstractG2Device.getJSON(method, payload): the result or an error.
func (d *Device) call(ctx context.Context, method string, params *ojson.Value) (*ojson.Value, error) {
	var p any = params
	if params == nil {
		p = ojson.NewObject()
	}
	b, err := d.Conn.Call(ctx, method, p)
	if err != nil {
		return nil, d.note(err)
	}
	if len(b) == 0 {
		return ojson.NewObject(), nil
	}
	return ojson.Parse(b)
}

// paged is AbstractG2Device.getPagedJson: follows offset/total and merges
// the arrayKey arrays into the first answer.
func (d *Device) paged(ctx context.Context, path, arrayKey string) (*ojson.Value, error) {
	first, err := d.get(ctx, path)
	if err != nil {
		return nil, err
	}
	arr := first.Get(arrayKey)
	total := first.Get("total").Int()
	if !first.Get("offset").Exists() || total <= 0 || arr.Kind() != ojson.Array {
		return first, nil
	}
	offset := first.Get("offset").Int() + arr.Len()
	for offset < total {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		next, err := d.get(ctx, path+sep+"offset="+strconv.Itoa(offset))
		if err != nil {
			return nil, err
		}
		items := next.Get(arrayKey).Items()
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			arr.Append(it)
		}
		offset += len(items)
	}
	return first, nil
}

func msgPtr(err error) *string {
	if err == nil {
		return nil
	}
	s := errorText(err)
	return &s
}

// errorText: the device's message, or the original's status markers.
func errorText(err error) string {
	var api *shelly.APIError
	switch {
	case errors.As(err, &api) && api.Message != "":
		return api.Message
	case shelly.IsOffline(err):
		return "Status-OFFLINE"
	case errors.Is(err, shelly.ErrUnauthorized):
		return "Status-PROTECTED"
	}
	return err.Error()
}

// Network is the connection a device uses now (WIFIManager.Network).
type Network int

// Networks.
const (
	NetUnknown Network = iota
	NetPrimary
	NetSecondary
	NetAP
	NetEthernet
)

// currentConnection: which configured network carries the device now.
func (d *Device) currentConnection(ctx context.Context) Network {
	if d.Gen == "1" {
		s, err := d.get(ctx, "/settings")
		if err != nil {
			return NetUnknown
		}
		switch {
		case s.Path("wifi_sta", "enabled").Bool() && s.Path("wifi_sta", "ssid").Text() == d.SSID:
			return NetPrimary
		case s.Path("wifi_sta1", "enabled").Bool() && s.Path("wifi_sta1", "ssid").Text() == d.SSID:
			return NetSecondary
		}
		return NetAP
	}
	c, err := d.get(ctx, "/rpc/Shelly.GetConfig")
	if err != nil {
		return NetUnknown
	}
	w := c.Get("wifi")
	switch {
	case w.Path("sta", "enable").Bool() && w.Path("sta", "ssid").Text() == d.SSID:
		return NetPrimary
	case w.Path("sta1", "enable").Bool() && w.Path("sta1", "ssid").Text() == d.SSID:
		return NetSecondary
	case w.Path("ap", "enable").Bool():
		return NetAP
	}
	return NetEthernet
}

// q is URLEncoder.encode (spaces as '+', like Java).
func q(s string) string { return url.QueryEscape(s) }
