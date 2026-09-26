// Package discovery finds Shelly devices on the LAN: mDNS browsing, IP-range
// scans, and the follow-ups from a found device (range extender, BLU).
//
// mDNS, as in ShellyScanner (model/Devices.java, JmDNS): browse the service
// type _http._tcp.local. and hand every resolved instance (name, IPv4, port) to
// the caller, which probes GET /shelly to decide whether it is a Shelly.
// ShellyScanner's _shelly._tcp listener is commented out; so is ours.
//
// This is a small browse-only implementation (no announcing, IPv4 only) on
// golang.org/x/net, chosen in Phase 2 over the available libraries because it
// needs nothing else and can be tested with constructed packets
// (DECISIONS.md §1.3).
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

// ServiceType is the mDNS service ShellyScanner browses.
const ServiceType = "_http._tcp.local."

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Instance is a resolved mDNS service instance.
type Instance struct {
	Name string // instance label, e.g. "shellyplus1-aabbcc000001"
	IP   net.IP
	Port int
}

// records accumulates what the network told us, across packets.
type records struct {
	mu       sync.Mutex
	ptr      map[string]bool                   // instance FQDNs of ServiceType
	srv      map[string]dnsmessage.SRVResource // instance FQDN → target, port
	a        map[string]net.IP                 // host FQDN → IPv4
	reported map[string]string                 // instance → "ip:port" last reported
	asked    map[string]time.Time              // instance → last follow-up query
}

func newRecords() *records {
	return &records{
		ptr:      map[string]bool{},
		srv:      map[string]dnsmessage.SRVResource{},
		a:        map[string]net.IP{},
		reported: map[string]string{},
		asked:    map[string]time.Time{},
	}
}

// ingest parses one mDNS packet and returns instances that became resolved or
// changed address, plus instance names that still lack SRV or A records.
func (r *records) ingest(pkt []byte) (resolved []Instance, missing []string, err error) {
	var p dnsmessage.Parser
	hdr, err := p.Start(pkt)
	if err != nil {
		return nil, nil, err
	}
	if !hdr.Response {
		return nil, nil, nil // somebody else's query
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Answers and additionals both carry useful records; Shelly devices put
	// SRV and A in the additional section of their PTR answer.
	for {
		h, err := p.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if err := r.record(&p, h, p.SkipAnswer); err != nil {
			return nil, nil, err
		}
	}
	if err := p.SkipAllAuthorities(); err == nil {
		for {
			h, err := p.AdditionalHeader()
			if err != nil {
				break // ErrSectionDone or a truncated tail: keep what we have
			}
			if err := r.record(&p, h, p.SkipAdditional); err != nil {
				break
			}
		}
	}
	now := time.Now()
	for inst := range r.ptr {
		srv, ok := r.srv[inst]
		var ip net.IP
		if ok {
			ip, ok = r.a[strings.ToLower(srv.Target.String())]
		}
		if !ok {
			if now.Sub(r.asked[inst]) > 5*time.Second { // ask again, but not for every packet
				r.asked[inst] = now
				missing = append(missing, inst)
			}
			continue
		}
		key := fmt.Sprintf("%s:%d", ip, srv.Port)
		if r.reported[inst] == key {
			continue
		}
		r.reported[inst] = key
		resolved = append(resolved, Instance{Name: instanceLabel(inst), IP: ip, Port: int(srv.Port)})
	}
	return resolved, missing, nil
}

// skip skips an unwanted record in the current section (SkipAnswer and
// SkipAdditional only work in their own section).
func (r *records) record(p *dnsmessage.Parser, h dnsmessage.ResourceHeader, skip func() error) error {
	name := strings.ToLower(h.Name.String())
	switch h.Type {
	case dnsmessage.TypePTR:
		ptr, err := p.PTRResource()
		if err != nil {
			return err
		}
		if name == ServiceType {
			inst := strings.ToLower(ptr.PTR.String())
			if h.TTL == 0 { // goodbye packet
				delete(r.ptr, inst)
				delete(r.reported, inst)
			} else {
				r.ptr[inst] = true
			}
		}
	case dnsmessage.TypeSRV:
		srv, err := p.SRVResource()
		if err != nil {
			return err
		}
		r.srv[name] = srv
	case dnsmessage.TypeA:
		a, err := p.AResource()
		if err != nil {
			return err
		}
		r.a[name] = net.IP(a.A[:]).To4()
	default:
		return skip()
	}
	return nil
}

// instanceLabel turns "shellyplus1-xyz._http._tcp.local." into "shellyplus1-xyz".
// DNS-SD instance labels may contain escaped dots and spaces ("Pro\ RGBWW").
func instanceLabel(fqdn string) string {
	label := strings.TrimSuffix(fqdn, "."+ServiceType)
	label = strings.TrimSuffix(label, ServiceType)
	return strings.NewReplacer(`\ `, " ", `\.`, ".", `\\`, `\`).Replace(label)
}

// query builds an mDNS query for the given names/types.
func query(qs ...dnsmessage.Question) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	for _, q := range qs {
		if err := b.Question(q); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

func mustName(s string) dnsmessage.Name {
	n, err := dnsmessage.NewName(s)
	if err != nil {
		panic(err)
	}
	return n
}

// Browser browses ServiceType on a set of interfaces until its context ends.
type Browser struct {
	// Interfaces to use; empty means every up, multicast-capable interface
	// with an IPv4 address ("full scan" in ShellyScanner's terms).
	Interfaces []string
	Log        *slog.Logger

	conn *ipv4.PacketConn
	rec  *records
	ifis []net.Interface
	mu   sync.Mutex
}

// Run browses and calls found for every resolved instance (again when its
// address changes). It returns when ctx is done or the socket cannot be opened.
func (b *Browser) Run(ctx context.Context, found func(Instance)) error {
	if b.Log == nil {
		b.Log = slog.Default()
	}
	ifis, err := selectInterfaces(b.Interfaces)
	if err != nil {
		return err
	}
	if len(ifis) == 0 {
		return errors.New("mdns: no usable network interface")
	}
	lc := net.ListenConfig{Control: reuseControl}
	pc, err := lc.ListenPacket(ctx, "udp4", "0.0.0.0:5353")
	if err != nil {
		return fmt.Errorf("mdns: %w", err)
	}
	defer pc.Close()
	conn := ipv4.NewPacketConn(pc)
	joined := 0
	for i := range ifis {
		if err := conn.JoinGroup(&ifis[i], &net.UDPAddr{IP: mdnsGroup.IP}); err != nil {
			b.Log.Debug("mdns: join failed", "interface", ifis[i].Name, "err", err)
			continue
		}
		joined++
	}
	if joined == 0 {
		return errors.New("mdns: could not join the multicast group on any interface")
	}
	_ = conn.SetMulticastTTL(255)
	_ = conn.SetMulticastLoopback(true)
	b.mu.Lock()
	b.conn, b.rec, b.ifis = conn, newRecords(), ifis
	b.mu.Unlock()
	b.Log.Info("mdns: browsing", "service", ServiceType, "interfaces", names(ifis))

	go func() { <-ctx.Done(); pc.Close() }()
	go b.queryLoop(ctx)

	buf := make([]byte, 9000)
	for {
		n, _, src, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("mdns: read: %w", err)
		}
		resolved, missing, err := b.rec.ingest(buf[:n])
		if err != nil {
			b.Log.Debug("mdns: bad packet", "from", src, "err", err)
			continue
		}
		for _, inst := range resolved {
			found(inst)
		}
		if len(missing) > 0 {
			b.resolve(missing)
		}
	}
}

// Query asks the network again now (used by Rescan).
func (b *Browser) Query() {
	b.send(dnsmessage.Question{Name: mustName(ServiceType), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
}

// Forget clears what was reported, so a following Query reports every instance again.
func (b *Browser) Forget() {
	b.mu.Lock()
	rec := b.rec
	b.mu.Unlock()
	if rec != nil {
		rec.mu.Lock()
		rec.reported = map[string]string{}
		rec.mu.Unlock()
	}
}

// queryLoop repeats the query at growing intervals, as mDNS browsers do
// (RFC 6762 §5.2): 0 s, 1 s, 3 s, 7 s … capped at 5 minutes.
func (b *Browser) queryLoop(ctx context.Context) {
	wait := time.Second
	for {
		b.Query()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > 5*time.Minute {
			wait = 5 * time.Minute
		}
	}
}

func (b *Browser) resolve(instances []string) {
	var qs []dnsmessage.Question
	for _, inst := range instances {
		qs = append(qs, dnsmessage.Question{Name: mustName(inst), Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET})
	}
	b.send(qs...)
}

func (b *Browser) send(qs ...dnsmessage.Question) {
	b.mu.Lock()
	conn, ifis := b.conn, b.ifis
	b.mu.Unlock()
	if conn == nil {
		return
	}
	pkt, err := query(qs...)
	if err != nil {
		b.Log.Error("mdns: build query", "err", err)
		return
	}
	for i := range ifis {
		if err := conn.SetMulticastInterface(&ifis[i]); err != nil {
			continue
		}
		if _, err := conn.WriteTo(pkt, nil, mdnsGroup); err != nil {
			b.Log.Debug("mdns: send failed", "interface", ifis[i].Name, "err", err)
		}
	}
}

// Interface describes a network interface for the scan settings.
type Interface struct {
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

// Interfaces lists up, multicast-capable interfaces with an IPv4 address,
// without container bridges.
func Interfaces() ([]Interface, error) {
	ifis, err := selectInterfaces(nil)
	if err != nil {
		return nil, err
	}
	out := make([]Interface, 0, len(ifis))
	for _, ifi := range ifis {
		out = append(out, Interface{Name: ifi.Name, Addrs: ipv4Addrs(ifi)})
	}
	return out, nil
}

func selectInterfaces(want []string) ([]net.Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.Interface
	for _, ifi := range all {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(ipv4Addrs(ifi)) == 0 {
			continue
		}
		if len(want) > 0 && !contains(want, ifi.Name) {
			continue
		}
		if len(want) == 0 && virtual(ifi.Name) {
			continue // container bridges never have Shellies behind them
		}
		out = append(out, ifi)
	}
	return out, nil
}

func ipv4Addrs(ifi net.Interface) []string {
	addrs, _ := ifi.Addrs()
	var out []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			out = append(out, ipn.String())
		}
	}
	return out
}

// virtual reports container and VM bridge interfaces, skipped by the default
// "all interfaces" scan (they can still be chosen explicitly).
func virtual(name string) bool {
	for _, p := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "podman", "vnet"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func names(ifis []net.Interface) []string {
	var out []string
	for _, i := range ifis {
		out = append(out, i.Name)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
