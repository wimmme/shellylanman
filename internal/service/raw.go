package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/wimmme/shellylanman/internal/model"
)

// Gen1ReadPaths are the read-only Gen1 endpoints DeviceGet may fetch.
var Gen1ReadPaths = []string{"/shelly", "/status", "/settings", "/settings/actions", "/ota"}

// DeviceGet reads one Gen1 endpoint of Gen1ReadPaths as the device answers it
// (the Gen2+ equivalent is DeviceRPC with a Get method).
func (m *Devices) DeviceGet(ctx context.Context, id, path string) (json.RawMessage, error) {
	if !slices.Contains(Gen1ReadPaths, path) {
		return nil, fmt.Errorf("%w: %s is not a Gen1 read endpoint", ErrBadCommand, path)
	}
	e, err := m.entryFor(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	d, conn := e.dev, e.conn
	m.mu.Unlock()
	switch {
	case d.Gen != "1":
		return nil, fmt.Errorf("%w: Gen1 devices only", ErrBadCommand)
	case conn == nil || d.Status == model.StatusGhost:
		return nil, ErrNoConnection
	}
	b, err := conn.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}
