// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// AbstractG2Device.executeRPC/postCommand and LoginManagerG2.getAuthNode.

package shelly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Call runs a Gen2+ RPC method with POST /rpc and returns its "result".
//
// Like ShellyScanner (AbstractG2Device.executeRPC), a protected device first
// answers 401 with the challenge as a JSON string in "message"; the request
// is repeated once with the JSON-RPC "auth" object
// (https://shelly-api-docs.shelly.cloud/gen2/General/Authentication).
func (d *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if wait := Pacing - time.Since(d.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { d.last = time.Now() }()

	if params == nil {
		params = struct{}{}
	}
	req := map[string]any{"id": 1, "method": method, "params": params}
	b, status, err := d.post(ctx, req)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized && d.cred != nil {
		auth, ok := rpcAuth(b, d.cred.Password)
		if !ok {
			return nil, ErrUnauthorized
		}
		time.Sleep(Pacing)
		req["auth"] = auth
		if b, status, err = d.post(ctx, req); err != nil {
			return nil, err
		}
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		if len(b) == 0 {
			return nil, &APIError{HTTPStatus: status}
		}
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	if resp.Error != nil {
		msg := resp.Error.Message
		if msg == "" {
			msg = "Generic error"
		}
		return nil, &APIError{HTTPStatus: status, Code: resp.Error.Code, Message: msg}
	}
	return resp.Result, nil
}

func (d *Conn) post(ctx context.Context, body any) ([]byte, int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+d.addr+"/rpc", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.HTTP.Do(req)
	if err != nil {
		return nil, 0, &OfflineError{err}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, &OfflineError{err}
	}
	return b, resp.StatusCode, nil
}

// rpcAuth builds the "auth" object from a 401 answer
// {"code":401,"message":"{\"auth_type\":\"digest\",\"nonce\":…,\"realm\":…,\"algorithm\":\"SHA-256\"}"}
// (LoginManagerG2.getAuthNode: the challenge fields plus cnonce, response
// and username; nc defaults to 1).
func rpcAuth(body []byte, password string) (map[string]any, bool) {
	var outer struct {
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(body, &outer) != nil || len(outer.Message) == 0 {
		return nil, false
	}
	var ch map[string]any
	var inner string
	if json.Unmarshal(outer.Message, &inner) == nil {
		if json.Unmarshal([]byte(inner), &ch) != nil {
			return nil, false
		}
	} else if json.Unmarshal(outer.Message, &ch) != nil {
		return nil, false
	}
	realm, _ := ch["realm"].(string)
	nonce := jsonScalar(ch["nonce"])
	if realm == "" || nonce == "" {
		return nil, false
	}
	nc := "1"
	if v, ok := ch["nc"]; ok {
		nc = jsonScalar(v)
		delete(ch, "nc")
	}
	cnonce := strconv.Itoa(rand.IntN(1 << 30))
	ch["cnonce"] = cnonce
	ch["response"] = RPCAuthResponse(realm, password, nonce, nc, cnonce)
	ch["username"] = DigestUser
	return ch, true
}

func jsonScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}
