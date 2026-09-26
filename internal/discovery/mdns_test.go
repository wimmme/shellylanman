package discovery

import (
	"context"
	"net"
	"reflect"
	"sync"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

type rr struct {
	name string
	body dnsmessage.ResourceBody
	ttl  uint32
}

func packet(t *testing.T, response bool, answers, additionals []rr) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: response, Authoritative: response})
	b.EnableCompression()
	add := func(section func(dnsmessage.ResourceHeader, dnsmessage.ResourceBody) error, r rr) {
		ttl := r.ttl
		if ttl == 0 {
			ttl = 120
		}
		if r.ttl == 1<<31 { // goodbye marker in tests
			ttl = 0
		}
		h := dnsmessage.ResourceHeader{Name: mustName(r.name), Class: dnsmessage.ClassINET, TTL: ttl}
		if err := section(h, r.body); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(b.StartAnswers())
	for _, r := range answers {
		add(func(h dnsmessage.ResourceHeader, body dnsmessage.ResourceBody) error {
			return addBody(&b, h, body, false)
		}, r)
	}
	must(b.StartAdditionals())
	for _, r := range additionals {
		add(func(h dnsmessage.ResourceHeader, body dnsmessage.ResourceBody) error {
			return addBody(&b, h, body, true)
		}, r)
	}
	out, err := b.Finish()
	must(err)
	return out
}

func addBody(b *dnsmessage.Builder, h dnsmessage.ResourceHeader, body dnsmessage.ResourceBody, _ bool) error {
	switch v := body.(type) {
	case *dnsmessage.PTRResource:
		return b.PTRResource(h, *v)
	case *dnsmessage.SRVResource:
		return b.SRVResource(h, *v)
	case *dnsmessage.AResource:
		return b.AResource(h, *v)
	case *dnsmessage.TXTResource:
		return b.TXTResource(h, *v)
	}
	panic("unsupported")
}

const inst = "shellyplus1-aabbcc000001._http._tcp.local."
const host = "shellyplus1-aabbcc000001.local."

func shellyAnswer(t *testing.T, ip [4]byte, port uint16) []byte {
	return packet(t, true,
		[]rr{{name: ServiceType, body: &dnsmessage.PTRResource{PTR: mustName(inst)}}},
		[]rr{
			{name: inst, body: &dnsmessage.SRVResource{Target: mustName(host), Port: port}},
			{name: inst, body: &dnsmessage.TXTResource{TXT: []string{"gen=2", "app=Plus1"}}},
			{name: host, body: &dnsmessage.AResource{A: ip}},
		})
}

func TestIngestResolvesPTRWithAdditionals(t *testing.T) {
	r := newRecords()
	got, missing, err := r.ingest(shellyAnswer(t, [4]byte{192, 0, 2, 10}, 80))
	if err != nil || len(missing) != 0 {
		t.Fatalf("ingest: %v, missing %v", err, missing)
	}
	want := []Instance{{Name: "shellyplus1-aabbcc000001", IP: net.IPv4(192, 0, 2, 10).To4(), Port: 80}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	// Same answer again: nothing new. New address: reported again.
	if got, _, _ := r.ingest(shellyAnswer(t, [4]byte{192, 0, 2, 10}, 80)); len(got) != 0 {
		t.Fatalf("duplicate reported: %+v", got)
	}
	if got, _, _ := r.ingest(shellyAnswer(t, [4]byte{192, 0, 2, 11}, 80)); len(got) != 1 || !got[0].IP.Equal(net.IPv4(192, 0, 2, 11)) {
		t.Fatalf("address change not reported: %+v", got)
	}
}

func TestIngestAsksForMissingRecords(t *testing.T) {
	r := newRecords()
	ptrOnly := packet(t, true, []rr{{name: ServiceType, body: &dnsmessage.PTRResource{PTR: mustName(inst)}}}, nil)
	got, missing, err := r.ingest(ptrOnly)
	if err != nil || len(got) != 0 || len(missing) != 1 || missing[0] != inst {
		t.Fatalf("got %v missing %v err %v", got, missing, err)
	}
	// Not asked again for every packet.
	if _, missing, _ := r.ingest(ptrOnly); len(missing) != 0 {
		t.Fatalf("asked again immediately: %v", missing)
	}
	// SRV and A arriving in a separate answer complete it.
	rest := packet(t, true, []rr{
		{name: inst, body: &dnsmessage.SRVResource{Target: mustName(host), Port: 8080}},
		{name: host, body: &dnsmessage.AResource{A: [4]byte{192, 0, 2, 20}}},
	}, nil)
	got, _, _ = r.ingest(rest)
	if len(got) != 1 || got[0].Port != 8080 {
		t.Fatalf("not resolved after SRV/A: %+v", got)
	}
}

func TestIngestIgnoresQueriesAndOtherServices(t *testing.T) {
	r := newRecords()
	q, _ := query(dnsmessage.Question{Name: mustName(ServiceType), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
	if got, _, err := r.ingest(q); err != nil || len(got) != 0 {
		t.Fatalf("query ingested: %v %v", got, err)
	}
	other := packet(t, true, []rr{{name: "_ipp._tcp.local.", body: &dnsmessage.PTRResource{PTR: mustName("printer._ipp._tcp.local.")}}}, nil)
	if got, missing, _ := r.ingest(other); len(got) != 0 || len(missing) != 0 {
		t.Fatalf("other service ingested: %v %v", got, missing)
	}
	if _, _, err := r.ingest([]byte{1, 2, 3}); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestGoodbyeForgetsInstance(t *testing.T) {
	r := newRecords()
	r.ingest(shellyAnswer(t, [4]byte{192, 0, 2, 10}, 80))
	bye := packet(t, true, []rr{{name: ServiceType, body: &dnsmessage.PTRResource{PTR: mustName(inst)}, ttl: 1 << 31}}, nil)
	r.ingest(bye)
	// Coming back is reported again.
	if got, _, _ := r.ingest(shellyAnswer(t, [4]byte{192, 0, 2, 10}, 80)); len(got) != 1 {
		t.Fatalf("instance after goodbye not reported: %+v", got)
	}
}

func TestInstanceLabelUnescapes(t *testing.T) {
	if got := instanceLabel(`Pro\ RGBWW._http._tcp.local.`); got != "Pro RGBWW" {
		t.Fatalf("label %q", got)
	}
}

func TestRangeValidationAndAddresses(t *testing.T) {
	good := Range{Base: "192.168.1", First: 250, Last: 252}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Range{{Base: "192.168", First: 1, Last: 2}, {Base: "192.168.300", First: 1, Last: 2}, {Base: "10.0.0", First: 9, Last: 3}, {Base: "10.0.0", First: 1, Last: 256}} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	got := Addresses([]Range{good, {Base: "10.0.0", First: 7, Last: 7}})
	want := []string{"192.168.1.250", "192.168.1.251", "192.168.1.252", "10.0.0.7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("addresses %v", got)
	}
}

func TestIPScanProbesEveryAddress(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	IPScan(context.Background(), []Range{{Base: "192.0.2", First: 1, Last: 20}}, 8080, 4, func(_ context.Context, addr string) {
		mu.Lock()
		seen[addr] = true
		mu.Unlock()
	})
	if len(seen) != 20 || !seen["192.0.2.20:8080"] {
		t.Fatalf("probed %d addresses", len(seen))
	}
}
