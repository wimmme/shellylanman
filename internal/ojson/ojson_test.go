package ojson

import "testing"

func TestOrderAndLiterals(t *testing.T) {
	v := MustParse(`{"z":1,"a":{"y":2.50,"b":[true,null,"x"]},"m":-85}`)
	if got := v.String(); got != `{"z":1,"a":{"y":2.50,"b":[true,null,"x"]},"m":-85}` {
		t.Fatalf("round trip %s", got)
	}
	if k := v.Keys(); k[0] != "z" || k[1] != "a" || k[2] != "m" {
		t.Fatalf("keys %v", k)
	}
	if v.Path("a", "y").Text() != "2.50" || v.Get("m").Text() != "-85" || v.Path("a", "b").Idx(0).Text() != "true" || v.Get("a").Text() != "" {
		t.Fatal("Text like Jackson asString")
	}
	if v.Get("nope").Exists() || !v.Path("a", "b").Idx(1).IsNull() || v.Path("a", "b").Idx(1).NonNull() {
		t.Fatal("missing / null")
	}
	c := v.Clone()
	c.Remove("z")
	c.Set("q", Str("new"))
	c.Get("a").Set("y", Int(3))
	if v.String() == c.String() || c.String() != `{"a":{"y":3,"b":[true,null,"x"]},"m":-85,"q":"new"}` {
		t.Fatalf("clone %s / %s", v, c)
	}
	if !Equal(MustParse(`{"a":1,"b":[1,2]}`), MustParse(`{"b":[1,2.0],"a":1}`)) || Equal(MustParse(`[1,2]`), MustParse(`[2,1]`)) {
		t.Fatal("Equal")
	}
	if Obj("id", 0, "config", MustParse(`{"x":true}`), "n", nil).String() != `{"id":0,"config":{"x":true},"n":null}` {
		t.Fatal("Obj")
	}
}
