// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// controller/DeferrableTask and controller/DeferrablesContainer.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

// Deferred task types (DeferrableTask.Type; FW_UPDATE, RESTORE and BACKUP
// arrive with their phases).
const (
	TaskMQTT       = "MQTT"
	TaskLogin      = "LOGIN"
	TaskNTP        = "NTP"
	TaskCloud      = "CLOUD_ENABLE"
	TaskInputReset = "INPUT_RESET_ENABLE"
)

// Deferred task states (DeferrableTask.Status).
const (
	DefWaiting   = "WAITING"
	DefCancelled = "CANCELLED"
	DefRunning   = "RUNNING"
	DefSuccess   = "SUCCESS"
	DefFail      = "FAIL"
)

// EventDeferred carries the full list of deferred tasks to the browsers.
const EventDeferred = "deferred.changed"

// maxDeferred bounds the persisted list: finished tasks beyond it are dropped
// oldest first (the original keeps them until the program exits).
const maxDeferred = 200

// DeferredTask is one queued action for a device that was off line.
type DeferredTask struct {
	ID          string `json:"id"`
	Time        int64  `json:"time"` // unix ms when it was queued
	DeviceID    string `json:"deviceId"`
	DeviceName  string `json:"deviceName"`
	Type        string `json:"type"`
	Description string `json:"description"` // i18n key: mqttEnable, mqttDisable, loginEnable, loginDisable, ntp, cloud, reset
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	Sealed      string `json:"sealed"` // the action's parameters (passwords included), encrypted
}

type deferredFile struct {
	Version int            `json:"version"`
	Tasks   []DeferredTask `json:"tasks"`
}

type deferredQueue struct {
	mu    sync.Mutex
	tasks []DeferredTask
	seq   int
}

func (m *Devices) loadDeferred() {
	var f deferredFile
	if _, err := m.store.LoadJSON(store.DeferredFile, &f); err != nil {
		m.log.Error("deferred", "err", err)
	}
	q := &m.deferred
	q.mu.Lock()
	for i := range f.Tasks {
		if f.Tasks[i].Status == DefRunning { // interrupted by a restart
			f.Tasks[i].Status, f.Tasks[i].Message = DefFail, "interrupted"
		}
		if n, err := strconv.Atoi(f.Tasks[i].ID); err == nil && n > q.seq {
			q.seq = n
		}
	}
	q.tasks = f.Tasks
	q.mu.Unlock()
}

// saveDeferred writes the list and tells the browsers; callers do not hold q.mu.
func (m *Devices) saveDeferred() {
	q := &m.deferred
	q.mu.Lock()
	list := append([]DeferredTask(nil), q.tasks...)
	q.mu.Unlock()
	if err := m.store.SaveJSON(store.DeferredFile, deferredFile{Version: 1, Tasks: list}); err != nil {
		m.log.Error("deferred save", "err", err)
	}
	m.emit(EventDeferred, publicDeferred(list))
}

func publicDeferred(list []DeferredTask) []DeferredTask {
	out := make([]DeferredTask, len(list))
	for i, t := range list {
		t.Sealed = ""
		out[i] = t
	}
	return out
}

// Deferred returns the task list (without parameters), oldest first.
func (m *Devices) Deferred() []DeferredTask {
	q := &m.deferred
	q.mu.Lock()
	defer q.mu.Unlock()
	return publicDeferred(q.tasks)
}

func describe(typ string, params any) string {
	switch p := params.(type) {
	case MQTTApply:
		if p.Enabled {
			return "mqttEnable"
		}
		return "mqttDisable"
	case fwParams:
		if p.Stable {
			return "fwStable"
		}
		return "fwBeta"
	case LoginApply:
		if p.Enabled {
			return "loginEnable"
		}
		return "loginDisable"
	}
	return map[string]string{TaskNTP: "ntp", TaskCloud: "cloud", TaskInputReset: "reset", TaskBackup: "backup", TaskRestore: "restore"}[typ]
}

// defer_ queues an action (DeferrablesContainer.addOrUpdate): a waiting task
// of the same type for the same device is cancelled first.
func (m *Devices) defer_(t cfgTarget, typ string, params any) {
	raw, _ := json.Marshal(params)
	q := &m.deferred
	q.mu.Lock()
	q.seq++
	id := strconv.Itoa(q.seq)
	q.mu.Unlock()
	sealed, err := m.store.Seal("deferred:"+id, string(raw))
	if err != nil {
		m.log.Error("deferred seal", "err", err)
		return
	}
	q.mu.Lock()
	for i := range q.tasks {
		if q.tasks[i].DeviceID == t.d.ID && q.tasks[i].Type == typ && q.tasks[i].Status == DefWaiting {
			q.tasks[i].Status, q.tasks[i].Sealed = DefCancelled, ""
		}
	}
	q.tasks = append(q.tasks, DeferredTask{ID: id, Time: time.Now().UnixMilli(), DeviceID: t.d.ID, DeviceName: descName(t.d),
		Type: typ, Description: describe(typ, params), Status: DefWaiting, Sealed: sealed})
	// Drop the oldest finished tasks beyond the limit.
	for len(q.tasks) > maxDeferred {
		drop := -1
		for i, x := range q.tasks {
			if x.Status != DefWaiting && x.Status != DefRunning {
				drop = i
				break
			}
		}
		if drop < 0 {
			break
		}
		q.tasks = append(q.tasks[:drop], q.tasks[drop+1:]...)
	}
	q.mu.Unlock()
	m.saveDeferred()
}

// descName: UtilMiscellaneous.getDescName — the name, else the host name.
func descName(d model.Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Hostname
}

// CancelDeferred cancels a waiting task.
func (m *Devices) CancelDeferred(id string) error {
	q := &m.deferred
	q.mu.Lock()
	found := false
	for i := range q.tasks {
		if q.tasks[i].ID == id {
			found = true
			if q.tasks[i].Status != DefWaiting {
				q.mu.Unlock()
				return fmt.Errorf("%w: task is %s", ErrBadCommand, q.tasks[i].Status)
			}
			q.tasks[i].Status, q.tasks[i].Sealed = DefCancelled, ""
		}
	}
	q.mu.Unlock()
	if !found {
		return ErrNotFound
	}
	m.saveDeferred()
	return nil
}

// runDeferred starts the first waiting task of a device that is on line
// (DeferrablesContainer.update: one task per device update).
func (m *Devices) runDeferred(id string) {
	q := &m.deferred
	q.mu.Lock()
	idx := -1
	for i := range q.tasks {
		if q.tasks[i].DeviceID == id && q.tasks[i].Status == DefWaiting {
			idx = i
			break
		}
	}
	if idx < 0 {
		q.mu.Unlock()
		return
	}
	task := &q.tasks[idx]
	task.Status = DefRunning
	tid, typ, sealed := task.ID, task.Type, task.Sealed
	q.mu.Unlock()
	m.saveDeferred()

	go func() {
		msg := m.execDeferred(tid, id, typ, sealed)
		q.mu.Lock()
		for i := range q.tasks {
			if q.tasks[i].ID == tid {
				q.tasks[i].Sealed = ""
				if msg == "" {
					q.tasks[i].Status = DefSuccess
				} else {
					q.tasks[i].Status, q.tasks[i].Message = DefFail, msg
				}
				if d, ok := m.Get(id); ok {
					q.tasks[i].DeviceName = descName(d) // may have changed since it was queued
				}
			}
		}
		q.mu.Unlock()
		m.saveDeferred()
	}()
}

func (m *Devices) execDeferred(tid, id, typ, sealed string) string {
	raw, err := m.store.Unseal("deferred:"+tid, sealed)
	if err != nil {
		return err.Error()
	}
	if typ == TaskBackup || typ == TaskRestore || typ == TaskFWUpdate { // also BLU devices
		e, err := m.entryFor(id)
		if err != nil {
			return err.Error()
		}
		m.mu.Lock()
		ctx := m.run
		m.mu.Unlock()
		if ctx == nil {
			ctx = context.Background()
		}
		if typ == TaskFWUpdate {
			return m.runFWTask(ctx, e, raw)
		}
		return m.runBackupRestoreTask(ctx, e, typ, raw)
	}
	ts, err := m.targets([]string{id})
	if err != nil {
		return err.Error()
	}
	t := ts[0]
	if !t.usable() {
		return "Status-" + string(t.d.Status)
	}
	m.mu.Lock()
	ctx := m.run
	m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	switch typ {
	case TaskLogin:
		var a LoginApply
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return err.Error()
		}
		return msgOf(m.setLogin(ctx, t, a))
	case TaskMQTT:
		var a MQTTApply
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return err.Error()
		}
		return msgOf(setMQTT(ctx, t, a))
	case TaskNTP, TaskCloud, TaskInputReset:
		var a OthersApply
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return err.Error()
		}
		return msgOf(setOthers(ctx, t, a))
	}
	return "unknown task " + typ
}

// deferredWaiting reports whether a device has a waiting task (cheap check
// done on every device update).
func (m *Devices) deferredWaiting(id string) bool {
	q := &m.deferred
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		if t.DeviceID == id && t.Status == DefWaiting {
			return true
		}
	}
	return false
}
