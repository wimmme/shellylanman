package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPolicy(t *testing.T) {
	for pw, want := range map[string]error{
		"":           ErrTooShort,
		"Abcdefg":    ErrTooShort,
		"abcdefgh":   ErrNoCapital,
		"12345678!":  ErrNoCapital,
		"Abcdefgh":   nil,
		"Ébène-déjà": nil, // capitals beyond ASCII count
		"ÄÖÜäöü12":   nil,
		"短密码短密码短密A":  nil,
	} {
		if got := CheckPolicy(pw); got != want {
			t.Errorf("CheckPolicy(%q) = %v, want %v", pw, got, want)
		}
	}
}

func TestHashVerify(t *testing.T) {
	h, err := Hash("Correct horse 1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$600000$") || strings.Contains(h, "Correct") {
		t.Fatalf("hash %q", h)
	}
	if !Verify("Correct horse 1", h) {
		t.Error("right password refused")
	}
	for _, wrong := range []string{"correct horse 1", "Correct horse 1 ", ""} {
		if Verify(wrong, h) {
			t.Errorf("%q accepted", wrong)
		}
	}
	h2, _ := Hash("Correct horse 1")
	if h2 == h {
		t.Error("same salt twice")
	}
	for _, bad := range []string{"", "x", "pbkdf2-sha256$0$AA$AA", "md5$1$AA$AA", "pbkdf2-sha256$1$!!$AA"} {
		if Verify("x", bad) {
			t.Errorf("Verify against %q", bad)
		}
	}
}

func TestSessions(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var saved map[string]Session
	saves := 0
	s := NewSessions(nil)
	s.Now = func() time.Time { return now }
	s.Save = func(m map[string]Session) { saved = m; saves++ }

	id, err := s.Create(true)
	if err != nil || len(id) < 40 {
		t.Fatalf("id %q %v", id, err)
	}
	if _, ok := saved[id]; ok || len(saved) != 1 {
		t.Fatalf("the clear id is stored: %v", saved)
	}
	if ok, rem := s.Valid(id); !ok || !rem {
		t.Fatal("new session refused")
	}
	if ok, _ := s.Valid(id + "x"); ok {
		t.Fatal("unknown id accepted")
	}

	// Used again after 29 days: still valid, and the limit starts again.
	now = now.Add(29 * 24 * time.Hour)
	if ok, _ := s.Valid(id); !ok {
		t.Fatal("refused within 30 days")
	}
	now = now.Add(29 * 24 * time.Hour)
	if ok, _ := s.Valid(id); !ok {
		t.Fatal("idle time not renewed by use")
	}
	now = now.Add(31 * 24 * time.Hour)
	if ok, _ := s.Valid(id); ok || s.Len() != 0 {
		t.Fatal("valid after 31 idle days")
	}

	// A restart keeps sessions; End and EndAll remove them.
	a, _ := s.Create(false)
	b, _ := s.Create(true)
	s2 := NewSessions(saved)
	s2.Now = s.Now
	if ok, rem := s2.Valid(a); !ok || rem {
		t.Fatal("session lost on restart")
	}
	s2.End(a)
	if ok, _ := s2.Valid(a); ok {
		t.Fatal("ended session valid")
	}
	if ok, _ := s2.Valid(b); !ok {
		t.Fatal("other session ended too")
	}
	s2.EndAll()
	if ok, _ := s2.Valid(b); ok || s2.Len() != 0 {
		t.Fatal("EndAll left a session")
	}
	if saves == 0 {
		t.Fatal("never saved")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := NewLimiter()
	l.Now = func() time.Time { return now }
	for i := 1; i < FreeTries; i++ {
		if n, w := l.Fail("a"); n != i || w != 0 {
			t.Fatalf("try %d: wait %v", n, w)
		}
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 60 * time.Second, 60 * time.Second}
	for _, w := range want {
		_, got := l.Fail("a")
		if got != w || l.Wait("a") != w {
			t.Fatalf("wait %v, want %v", got, w)
		}
		if l.Wait("b") != 0 {
			t.Fatal("other address slowed down")
		}
		now = now.Add(got)
		if l.Wait("a") != 0 {
			t.Fatal("still waiting after the wait")
		}
	}
	l.Succeed("a")
	if _, w := l.Fail("a"); w != 0 {
		t.Fatal("success did not reset")
	}
}
