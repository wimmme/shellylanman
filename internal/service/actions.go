package service

import (
	"context"
	"errors"
	"log/slog"
)

// Every change ShellyLanMan makes, to a device or to its own data, is one log
// line per device (DECISIONS P26-1): "action <name>", at info when it worked or
// was queued and at warning when it failed, with the device, the result and who
// asked for it. Secrets (passwords, script code, KVS values, RPC parameters)
// never go into the line.

// Actor is who asked for an action: the source and, for a request, the client's
// address.
type Actor struct {
	Source string // see the Source* constants
	Addr   string
}

// Action sources.
const (
	SourceWeb         = "web"         // the web UI or the REST API
	SourceIngress     = "ingress"     // the web UI in Home Assistant's sidebar
	SourceMCP         = "mcp"         // an MCP client
	SourceIntegration = "integration" // the Home Assistant integration, on the app's loopback listener
	SourceDeferred    = "deferred"    // a queued task, run when its device came back
	SourceSystem      = "system"      // ShellyLanMan itself (follow-ups of an earlier action)
)

type actorKey struct{}

// WithActor marks ctx with who asked for what is done with it.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorOf returns who asked (SourceSystem when nobody did).
func ActorOf(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok {
		return a
	}
	return Actor{Source: SourceSystem}
}

// Results of an action, as in ResultLine.
const (
	resultTimeout   = "timeout"
	resultUnchanged = "unchanged"
)

// action logs one action on one device (dev.ID empty: not about a device).
// err nil is ok; attrs are extra key/value pairs.
func (m *Devices) action(ctx context.Context, name string, dev DeviceRef, err error, attrs ...any) {
	result := ResultOK
	if err != nil {
		result = ResultFail
	}
	m.actionResult(ctx, name, dev, result, msgOrEmpty(err), attrs...)
}

func msgOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return msgOf(err)
}

// actionResult logs an action with an explicit result (ok, queued, fail, …)
// and the device's or ShellyLanMan's message.
func (m *Devices) actionResult(ctx context.Context, name string, dev DeviceRef, result, msg string, attrs ...any) {
	level := slog.LevelInfo
	if result != ResultOK && result != ResultQueued {
		level = slog.LevelWarn
	}
	args := make([]any, 0, 12+len(attrs))
	if dev.ID != "" {
		args = append(args, "device", dev.ID)
		if dev.Name != "" {
			args = append(args, "name", dev.Name)
		}
	}
	args = append(args, attrs...)
	args = append(args, "result", result)
	if msg != "" {
		args = append(args, "error", msg)
	}
	a := ActorOf(ctx)
	args = append(args, "source", a.Source)
	if a.Addr != "" {
		args = append(args, "addr", a.Addr)
	}
	m.log.Log(context.Background(), level, "action "+name, args...)
}

// actionLines logs the result lines of an action on several devices.
func (m *Devices) actionLines(ctx context.Context, name string, lines []ResultLine, attrs ...any) {
	for _, l := range lines {
		m.actionResult(ctx, name, l.DeviceRef, l.Result, l.Message, attrs...)
	}
}

// refOf names device id for an action line (only the id when it is unknown).
func (m *Devices) refOf(id string) DeviceRef {
	if d, ok := m.Get(id); ok {
		return DeviceRef{ID: id, Name: descName(d)}
	}
	return DeviceRef{ID: id}
}

// stepsErr: the first failed step of a profile, as an error for the action log.
func stepsErr(steps []ProfileStep, err error) error {
	if err != nil {
		return err
	}
	for _, s := range steps {
		if s.Result == ResultFail {
			return errors.New(s.Step + ": " + s.Message)
		}
	}
	return nil
}

func scriptUpdateAttrs(sid int, name *string, enable *bool) []any {
	a := []any{"script", sid}
	if name != nil {
		a = append(a, "scriptName", *name)
	}
	if enable != nil {
		a = append(a, "enable", *enable)
	}
	return a
}
