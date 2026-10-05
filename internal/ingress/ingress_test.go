package ingress

import (
	"reflect"
	"testing"
)

func base() []Rule {
	return []Rule{
		{"hostname": "a.example.com", "service": "http://127.0.0.1:1", "originRequest": map[string]any{"noTLSVerify": true}},
		{"service": CatchAll},
	}
}

func TestAddNeverAltersExistingRoute(t *testing.T) {
	in := base()
	snapshot := clone(in)
	out, err := Add(in, "b.example.com", "http://127.0.0.1:2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, snapshot) {
		t.Fatal("Add mutated its input")
	}
	if !reflect.DeepEqual(out[0], in[0]) {
		t.Fatalf("route A changed: %v", out[0])
	}
	if got := host(out[1]); got != "b.example.com" {
		t.Fatalf("B not before catch-all: %v", out)
	}
	if !IsCatchAll(out[len(out)-1]) {
		t.Fatal("catch-all must stay last")
	}
}

func TestRemoveOnlyTouchesThatHost(t *testing.T) {
	r, _ := Add(base(), "b.example.com", "http://127.0.0.1:2")
	out, err := Remove(r, "b.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, base()) {
		t.Fatalf("after add+remove expected original, got %v", out)
	}
}

func TestDuplicateAndMissing(t *testing.T) {
	if _, err := Add(base(), "a.example.com", "http://x"); err != ErrExists {
		t.Fatalf("want ErrExists got %v", err)
	}
	if _, err := Remove(base(), "zzz.example.com"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}

func TestNormalizeAddsCatchAll(t *testing.T) {
	out, err := Add(nil, "a.example.com", "http://x")
	if err != nil || !IsCatchAll(out[len(out)-1]) || len(out) != 2 {
		t.Fatalf("%v %v", out, err)
	}
}
