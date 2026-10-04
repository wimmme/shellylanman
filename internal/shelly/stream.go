package shelly

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/coder/websocket"
)

// Notifications calls a Gen2+ RPC method over the device's RPC WebSocket and
// passes every notification (NotifyEvent, NotifyStatus, …) to on until ctx
// ends; it then returns nil. A protected device answers the call with 401 and a
// challenge; the call is repeated once with the JSON-RPC "auth" object, as for
// POST /rpc (https://shelly-api-docs.shelly.cloud/gen2/General/Authentication).
// Used for BTHome.StartDeviceDiscovery, whose results arrive only as events.
func (d *Conn) Notifications(ctx context.Context, method string, params any, on func(method string, params json.RawMessage)) error {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	c, _, err := websocket.Dial(dctx, "ws://"+d.addr+"/rpc", nil)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return &OfflineError{err}
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	if params == nil {
		params = struct{}{}
	}
	src := "shellylanman-" + strconv.Itoa(rand.IntN(1_000_000))
	send := func(id int, auth map[string]any) error {
		req := map[string]any{"id": id, "src": src, "method": method, "params": params}
		if auth != nil {
			req["auth"] = auth
		}
		b, _ := json.Marshal(req)
		return c.Write(ctx, websocket.MessageText, b)
	}
	if err := send(1, nil); err != nil {
		return &OfflineError{err}
	}
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return &OfflineError{err}
		}
		var m struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if m.Method != "" {
			on(m.Method, m.Params)
			continue
		}
		if len(m.Error) == 0 || string(m.Error) == "null" {
			continue
		}
		var e struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(m.Error, &e)
		if e.Code == 401 && m.ID == 1 && d.cred != nil {
			auth, ok := rpcAuth(m.Error, d.cred.Password)
			if !ok {
				return ErrUnauthorized
			}
			if err := send(2, auth); err != nil {
				return &OfflineError{err}
			}
			continue
		}
		if e.Code == 401 {
			return ErrUnauthorized
		}
		return &APIError{Code: e.Code, Message: e.Message}
	}
}
