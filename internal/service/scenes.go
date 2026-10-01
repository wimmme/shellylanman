package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wimmme/shellylanman/internal/parse"
)

// Scenes are named, ordered lists of device actions that run on request
// (DECISIONS P11-3, parity with the Shelly-MCP). They live in scenes.json in
// the data directory. Not part of ShellyScanner.

// SceneFile is the scenes' file in the data directory.
const SceneFile = "scenes.json"

// Scene limits.
const (
	MaxScenes       = 100
	MaxSceneActions = 50
)

// SceneAction is one step: a Command on a module of a device, or (Gen2+) an
// RPC method with parameters. Exactly one of Command and Method is set.
type SceneAction struct {
	Device  string          `json:"device"` // device ID
	Command *Command        `json:"command,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Scene is a named list of actions.
type Scene struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Actions     []SceneAction `json:"actions"`
	Updated     int64         `json:"updated"` // unix ms
}

// SceneStep is the outcome of one action of a run.
type SceneStep struct {
	Index  int    `json:"index"`
	Device string `json:"device"`
	Name   string `json:"name,omitempty"`
	Result string `json:"result"` // ResultOK or ResultFail
	Error  string `json:"error,omitempty"`
}

// SceneRun is the outcome of a run: Status "ok" (all steps), "partial" or "failed" (none).
type SceneRun struct {
	Scene  string      `json:"scene"`
	Status string      `json:"status"`
	Steps  []SceneStep `json:"steps"`
}

// Scene errors.
var (
	ErrSceneExists = errors.New("scene exists") // same name, overwrite not asked
	ErrNoScene     = errors.New("no such scene")
)

type sceneStore struct {
	mu     sync.Mutex
	loaded bool
	list   []Scene
}

type sceneFile struct {
	Version int     `json:"version"`
	Scenes  []Scene `json:"scenes"`
}

// load reads the file once; callers hold s.scenes.mu.
func (m *Devices) loadScenes() error {
	if m.scenes.loaded {
		return nil
	}
	var f sceneFile
	if _, err := m.store.LoadJSON(SceneFile, &f); err != nil {
		return err
	}
	m.scenes.list, m.scenes.loaded = f.Scenes, true
	return nil
}

func (m *Devices) saveScenes(list []Scene) error {
	if err := m.store.SaveJSON(SceneFile, sceneFile{Version: 1, Scenes: list}); err != nil {
		return err
	}
	m.scenes.list = list
	return nil
}

// Scenes returns all scenes, sorted by name.
func (m *Devices) Scenes() ([]Scene, error) {
	m.scenes.mu.Lock()
	defer m.scenes.mu.Unlock()
	if err := m.loadScenes(); err != nil {
		return nil, err
	}
	out := slices.Clone(m.scenes.list)
	slices.SortFunc(out, func(a, b Scene) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// Scene returns one scene by name (case-insensitive).
func (m *Devices) Scene(name string) (Scene, error) {
	list, err := m.Scenes()
	if err != nil {
		return Scene{}, err
	}
	for _, s := range list {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	return Scene{}, fmt.Errorf("%w: %q", ErrNoScene, name)
}

// SceneSave adds a scene, or replaces the one with the same name when overwrite is set.
func (m *Devices) SceneSave(sc Scene, overwrite bool) (Scene, error) {
	sc.Name = strings.TrimSpace(sc.Name)
	if err := m.validateScene(sc); err != nil {
		return Scene{}, err
	}
	sc.Updated = time.Now().UnixMilli()
	m.scenes.mu.Lock()
	defer m.scenes.mu.Unlock()
	if err := m.loadScenes(); err != nil {
		return Scene{}, err
	}
	list := slices.Clone(m.scenes.list)
	i := slices.IndexFunc(list, func(x Scene) bool { return strings.EqualFold(x.Name, sc.Name) })
	switch {
	case i >= 0 && !overwrite:
		return Scene{}, fmt.Errorf("%w: %q", ErrSceneExists, sc.Name)
	case i >= 0:
		list[i] = sc
	case len(list) >= MaxScenes:
		return Scene{}, invalid(fmt.Sprintf("at most %d scenes", MaxScenes))
	default:
		list = append(list, sc)
	}
	return sc, m.saveScenes(list)
}

// SceneDelete removes a scene.
func (m *Devices) SceneDelete(name string) error {
	m.scenes.mu.Lock()
	defer m.scenes.mu.Unlock()
	if err := m.loadScenes(); err != nil {
		return err
	}
	i := slices.IndexFunc(m.scenes.list, func(x Scene) bool { return strings.EqualFold(x.Name, name) })
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrNoScene, name)
	}
	return m.saveScenes(slices.Delete(slices.Clone(m.scenes.list), i, i+1))
}

func (m *Devices) validateScene(sc Scene) error {
	if sc.Name == "" || utf8.RuneCountInString(sc.Name) > 64 {
		return invalid("scene name (1–64 characters)")
	}
	if len(sc.Actions) == 0 || len(sc.Actions) > MaxSceneActions {
		return invalid(fmt.Sprintf("a scene needs 1–%d actions", MaxSceneActions))
	}
	for i, a := range sc.Actions {
		d, ok := m.Get(a.Device)
		if !ok {
			return invalid(fmt.Sprintf("action %d: unknown device %q", i+1, a.Device))
		}
		switch {
		case (a.Command == nil) == (a.Method == ""):
			return invalid(fmt.Sprintf("action %d: give a command or a method", i+1))
		case a.Command != nil:
			if a.Command.Action == ActionEvent || !slices.ContainsFunc(d.Modules, func(x parse.Module) bool { return x.Key == a.Command.Key && x.Key != "" }) {
				return invalid(fmt.Sprintf("action %d: %s has no module %q for this command", i+1, d.ID, a.Command.Key))
			}
		default:
			if d.Gen == "1" {
				return invalid(fmt.Sprintf("action %d: RPC methods need a Gen2+ device", i+1))
			}
			if c := ClassifyRPC(a.Method); c != RPCWrite {
				return invalid(fmt.Sprintf("action %d: %s is not allowed in a scene (%s)", i+1, a.Method, c))
			}
			if len(a.Params) > 0 && !json.Valid(a.Params) {
				return invalid(fmt.Sprintf("action %d: params is not JSON", i+1))
			}
		}
	}
	return nil
}

// SceneRunNow runs every action in order; a failing action does not stop the
// others (a partial run can be repeated: scenes should set absolute states).
func (m *Devices) SceneRunNow(ctx context.Context, name string) (SceneRun, error) {
	sc, err := m.Scene(name)
	if err != nil {
		return SceneRun{}, err
	}
	run := SceneRun{Scene: sc.Name}
	ok := 0
	for i, a := range sc.Actions {
		step := SceneStep{Index: i + 1, Device: a.Device, Result: ResultOK}
		if d, found := m.Get(a.Device); found {
			step.Name = descName(d)
		}
		var err error
		if a.Command != nil {
			err = m.Command(ctx, a.Device, *a.Command)
		} else {
			_, err = m.DeviceRPC(ctx, a.Device, a.Method, a.Params)
		}
		if err != nil {
			step.Result, step.Error = ResultFail, msgOf(err)
		} else {
			ok++
		}
		run.Steps = append(run.Steps, step)
	}
	switch ok {
	case len(sc.Actions):
		run.Status = "ok"
	case 0:
		run.Status = "failed"
	default:
		run.Status = "partial"
	}
	return run, nil
}
