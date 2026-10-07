package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

func testServer(t *testing.T) *server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, s := build(Config{Store: st, Hub: hub.New(nil, nil, nil), Static: fstest.MapFS{"index.html": {Data: []byte("x")}}})
	return s
}

// Every route of the server is in the description, and everything in the
// description is a route: a new endpoint without an entry fails here.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	s := testServer(t)
	var routes []string
	for _, p := range s.routes {
		if p == "GET /" { // the web page
			continue
		}
		routes = append(routes, p)
	}
	sort.Strings(routes)
	if got := operationKeys(); !reflect.DeepEqual(got, routes) {
		var missing, extra []string
		in := func(list []string, x string) bool {
			for _, y := range list {
				if x == y {
					return true
				}
			}
			return false
		}
		for _, r := range routes {
			if !in(got, r) {
				missing = append(missing, r)
			}
		}
		for _, g := range got {
			if !in(routes, g) {
				extra = append(extra, g)
			}
		}
		t.Fatalf("routes without a description: %v\ndescriptions without a route: %v", missing, extra)
	}
}

func TestOpenAPIWellFormed(t *testing.T) {
	doc := buildSpec()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back["openapi"] != "3.1.0" {
		t.Fatalf("version %v", back["openapi"])
	}
	ids := map[string]bool{}
	pathParam := regexp.MustCompile(`\{(\w+)\}`)
	for _, op := range operations {
		if op.id == "" || ids[op.id] {
			t.Errorf("%s %s: operation id %q missing or used twice", op.method, op.path, op.id)
		}
		ids[op.id] = true
		if op.summary == "" || op.tag == "" || op.ok.code == 0 || op.ok.desc == "" {
			t.Errorf("%s %s: summary, tag and the success response are needed", op.method, op.path)
		}
		if op.method == "GET" && op.body != nil {
			t.Errorf("%s %s: a GET has no body", op.method, op.path)
		}
		// Every {param} of the path is described.
		for _, m := range pathParam.FindAllStringSubmatch(op.path, -1) {
			if pathDocs[m[1]] == "" {
				t.Errorf("%s %s: path parameter %q has no description", op.method, op.path, m[1])
			}
		}
		// Anything that changes something and is not a plain read says what is wrong when it fails.
		if op.method != "GET" && len(op.errs) == 0 && op.auth == authDefault && op.body == nil && op.ok.code != 204 && op.ok.code != 202 {
			t.Errorf("%s %s: no error responses described", op.method, op.path)
		}
	}
	tags := map[string]bool{}
	for _, tg := range tagDocs {
		tags[tg.name] = true
	}
	for _, op := range operations {
		if !tags[op.tag] {
			t.Errorf("%s %s: tag %q is not in the tag list", op.method, op.path, op.tag)
		}
	}
	// Every $ref points at a component.
	comps := back["components"].(map[string]any)["schemas"].(map[string]any)
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if r, ok := x["$ref"].(string); ok {
				var found bool
				switch {
				case strings.HasPrefix(r, "#/components/schemas/"):
					found = comps[strings.TrimPrefix(r, "#/components/schemas/")] != nil
				case strings.HasPrefix(r, "#/components/responses/"):
					found = back["components"].(map[string]any)["responses"].(map[string]any)[strings.TrimPrefix(r, "#/components/responses/")] != nil
				}
				if !found {
					t.Errorf("$ref %s does not exist", r)
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(back)
}

// The schemas are made from the Go types: a field added to Device is in the description.
func TestOpenAPISchemasFollowTheTypes(t *testing.T) {
	g := newSchemaGen()
	g.of(model.Device{})
	dev := g.comps["Device"].(map[string]any)["properties"].(map[string]any)
	var got []string
	for k := range dev {
		got = append(got, k)
	}
	sort.Strings(got)
	if want := typeFields(reflect.TypeOf(model.Device{})); !reflect.DeepEqual(got, want) {
		t.Fatalf("Device properties %v, want %v", got, want)
	}
	for _, f := range []string{"id", "status", "updateAvailable", "rebootRequired", "modules"} {
		if dev[f] == nil {
			t.Errorf("Device has no %s", f)
		}
	}
	// A required field is one without omitempty; the status is an enum.
	req := g.comps["Device"].(map[string]any)["required"].([]string)
	if !contains(req, "id") || contains(req, "note") {
		t.Errorf("required fields of Device: %v", req)
	}
	if st := dev["status"].(map[string]any); st["enum"] == nil {
		t.Errorf("status is not an enum: %v", st)
	}
	// Two types called Status do not collide.
	full := buildSpec()["components"].(map[string]any)["schemas"].(map[string]any)
	for _, n := range []string{"Status", "UpdateStatus", "LogEntry", "Settings", "ResultLine", "Error"} {
		if full[n] == nil {
			t.Errorf("component %s missing", n)
		}
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestOpenAPIServed(t *testing.T) {
	srv, st := newTestServer(t, nil)
	var doc struct {
		OpenAPI string `json:"openapi"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	decode(t, do(t, "GET", srv.URL+"/api/v1/openapi.json", "", nil), &doc)
	if doc.OpenAPI != "3.1.0" || len(doc.Servers) != 1 || doc.Servers[0].URL != srv.URL {
		t.Fatalf("served: %s %+v (want server %s)", doc.OpenAPI, doc.Servers, srv.URL)
	}
	if _, ok := doc.Paths["/api/v1/devices/{id}"]["get"]; !ok {
		t.Fatal("GET /api/v1/devices/{id} is not described")
	}
	// Behind the same login as the rest of the API, reachable with the MCP token (DECISIONS P19-2).
	setPassword(t, st, "Secret pass")
	if r := do(t, "GET", srv.URL+"/api/v1/openapi.json", "", nil); r.StatusCode != 401 {
		t.Fatalf("without login: %d", r.StatusCode)
	}
	if _, err := st.Update(func(s *store.Settings) { s.MCP.Enabled = true; s.MCP.Access = "read" }); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret(store.MCPTokenSecret, "tok-1"); err != nil {
		t.Fatal(err)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/openapi.json", "", map[string]string{"Authorization": "Bearer tok-1"}); r.StatusCode != 200 {
		t.Fatalf("with the token: %d", r.StatusCode)
	}
}

// OPENAPI_OUT=file go test -run DumpOpenAPI ./internal/httpapi writes the description, to look at it
// or to run an external validator over it (docs/WORKFLOW.md).
func TestDumpOpenAPI(t *testing.T) {
	out := os.Getenv("OPENAPI_OUT")
	if out == "" {
		t.Skip("OPENAPI_OUT not set")
	}
	b, err := json.MarshalIndent(buildSpec(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
