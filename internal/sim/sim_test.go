package sim

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func start(t *testing.T, fixtureDir string) *httptest.Server {
	t.Helper()
	d, err := New(filepath.Join("..", "..", "testdata", fixtureDir))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	return srv
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func TestGen1PlugS(t *testing.T) {
	srv := start(t, "gen1/SHPLG-S")
	code, shelly := getJSON(t, srv.URL+"/shelly")
	if code != 200 || shelly["type"] != "SHPLG-S" || shelly["gen"] != nil {
		t.Fatalf("/shelly = %d %v", code, shelly)
	}
	code, status := getJSON(t, srv.URL+"/status")
	if code != 200 || status["relays"] == nil || status["meters"] == nil {
		t.Fatalf("/status = %d, keys missing", code)
	}
	if code, _ := getJSON(t, srv.URL+"/settings/actions"); code != 200 {
		t.Fatalf("/settings/actions = %d", code)
	}
	if code, _ := getJSON(t, srv.URL+"/nope"); code != 404 {
		t.Fatalf("unknown path = %d", code)
	}
}

func TestGen2Plus1GetAndPost(t *testing.T) {
	srv := start(t, "gen2/Plus1")
	code, shelly := getJSON(t, srv.URL+"/shelly")
	if code != 200 || shelly["app"] != "Plus1" || shelly["gen"] != float64(2) {
		t.Fatalf("/shelly = %d %v", code, shelly)
	}
	// GET form with a query string, as ShellyScanner uses it.
	code, info := getJSON(t, srv.URL+"/rpc/Shelly.GetDeviceInfo?ident=true")
	if code != 200 || info["app"] != "Plus1" {
		t.Fatalf("GetDeviceInfo = %d %v", code, info)
	}
	// POST form: result wrapped in an RPC envelope with src = device id.
	resp, err := http.Post(srv.URL+"/rpc", "application/json", strings.NewReader(`{"id":7,"method":"Shelly.GetStatus"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		ID     int            `json:"id"`
		Src    string         `json:"src"`
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.ID != 7 || env.Src != shelly["id"] || env.Result["sys"] == nil {
		t.Fatalf("envelope = %+v", env)
	}
	resp2, _ := http.Post(srv.URL+"/rpc", "application/json", strings.NewReader(`{"id":8,"method":"Nope.Nothing"}`))
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("unknown method = %d", resp2.StatusCode)
	}
}
