package local

import (
	"reflect"
	"testing"
)

func TestOrderedMapStoreAndLoad(t *testing.T) {
	om := newOrderedMap()
	om.Store("a", true)
	om.Store("b", false)
	om.Store("c", 24)
	om.Store("a", 32)

	for _, val := range []string{"a", "b", "c"} {
		if om.Contains(val) == false {
			t.Fatalf("Expected to contain %s", val)
		}
	}

	if om.Contains("nope") == true {
		t.Fatalf("Unexpected to contain nope")
	}

	val, ok := om.Load("a")
	if !ok {
		t.Fatalf("Not ok")
	}
	if val != 32 {
		t.Fatalf("Bad %v (%T)", val, val)
	}

	val, ok = om.Load("b")
	if !ok {
		t.Fatalf("Not ok")
	}
	if val != false {
		t.Fatalf("Bad %v (%T)", val, val)
	}

	val, ok = om.Load("c")
	if !ok {
		t.Fatalf("Not ok")
	}
	if val != 24 {
		t.Fatalf("Bad %v (%T)", val, val)
	}
}

func TestOrderedMapKeys(t *testing.T) {
	om := newOrderedMap()
	om.Store("a", true)
	om.Store("b", false)
	om.Store("c", 24)
	om.Store("a", 32)

	keys := om.Keys()

	if !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
		t.Fatalf("Unexpected keys: %#v", keys)
	}
}
