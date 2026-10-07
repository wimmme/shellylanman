package httpapi

import (
	"encoding/json"

	"github.com/wimmme/shellylanman/internal/sbk"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/store"
)

// The request bodies of the API, named so that the OpenAPI description
// (openapi.go) is made from the very types the handlers decode (DECISIONS P19-2).

type idsBody struct {
	IDs []string `json:"ids"`
}

// idsConfirmBody is for actions that need confirm=true (destructive, DECISIONS §8).
type idsConfirmBody struct {
	IDs     []string `json:"ids"`
	Confirm bool     `json:"confirm"`
}

type identifyBLUBody struct {
	Gateway  string `json:"gateway"`
	Duration int    `json:"duration"`
}

type pauseBody struct {
	Paused bool `json:"paused"`
}

type noteBody struct {
	Note    string  `json:"note"`
	Keyword string  `json:"keyword"`
	Name    *string `json:"name"` // relayed BLU devices only (DECISIONS P14-3)
}

type restoreCheckBody struct {
	Source service.RestoreSource `json:"source"`
}

type restoreBody struct {
	Source  service.RestoreSource `json:"source"`
	Answers sbk.Answers           `json:"answers"`
	Confirm bool                  `json:"confirm"`
}

type firmwareUpdateBody struct {
	Items   []service.FirmwareRequest `json:"items"`
	Confirm bool                      `json:"confirm"`
}

type scriptCreateBody struct {
	Name string `json:"name"`
}

type scriptUpdateBody struct {
	Name   *string `json:"name"`
	Enable *bool   `json:"enable"`
}

type scriptCodeBody struct {
	Code string `json:"code"`
}

type kvsBody struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// uploadBody carries an uploaded .sbk file, base64.
type uploadBody struct {
	Upload string `json:"upload"`
}

type rpcBody struct {
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Confirm bool            `json:"confirm"` // needed for risky methods (service.CheckRPC)
}

type loginBody struct {
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

type passwordBody struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

type mcpPatchBody struct {
	Enabled *bool   `json:"enabled"`
	Access  *string `json:"access"`
}

// settingsPatch is a partial update: only fields present in the body change.
type settingsPatch struct {
	FirstRunDone *bool                  `json:"firstRunDone"`
	Language     *string                `json:"language"`
	Scan         *store.ScanSettings    `json:"scan"`
	Archive      *store.ArchiveSettings `json:"archive"`
	MQTTSlow     *int                   `json:"mqttSlow"`
	BackupKeep   *int                   `json:"backupKeep"`
	PhoneBaseURL *string                `json:"phoneBaseURL"`
	UpdateCheck  *string                `json:"updateCheck"`
	SkipVersion  *string                `json:"skipVersion"`
}
