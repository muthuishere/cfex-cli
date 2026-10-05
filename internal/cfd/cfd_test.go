package cfd

import "testing"

func TestParseList(t *testing.T) {
	ts, err := ParseList([]byte(`[{"id":"a1","name":"one","conns":[{"id":"c"}]},{"id":"b2","name":"two","conns":[]}]`))
	if err != nil || len(ts) != 2 || !ts[0].Running() || ts[1].Running() {
		t.Fatalf("%v %v", ts, err)
	}
	for _, empty := range []string{"", "null", " null\n"} {
		if ts, err := ParseList([]byte(empty)); err != nil || ts != nil {
			t.Fatalf("%q -> %v %v", empty, ts, err)
		}
	}
	if _, err := ParseList([]byte("{")); err == nil {
		t.Fatal("bad json must error")
	}
}

func TestParseID(t *testing.T) {
	out := "Tunnel credentials written to /x/0dd3d6ba-3d8c-425f-8513-b97871c7c7d4.json\nCreated tunnel foo with id 0dd3d6ba-3d8c-425f-8513-b97871c7c7d4"
	if ParseID(out) != "0dd3d6ba-3d8c-425f-8513-b97871c7c7d4" || ParseID("nothing") != "" {
		t.Fatal("ParseID")
	}
}
