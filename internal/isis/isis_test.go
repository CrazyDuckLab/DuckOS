// isis/isis_test.go pins the V1 IS-IS
// configuration model and renderer.
package isis

import "testing"

func TestInstanceValid(t *testing.T) {
	good := Instance{Name: "core", NET: "49.0001.1920.6800.1001.00", Level: Level12, Metric: 10, SRGBStart: 16000, SRGBEnd: 23999}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	badNet := Instance{Name: "core", NET: "garbage", Level: Level12}
	if err := badNet.Valid(); err == nil {
		t.Fatal("bad NET must fail")
	}
	badLevel := Instance{Name: "core", NET: "49.0001.1920.6800.1001.00", Level: "L3"}
	if err := badLevel.Valid(); err == nil {
		t.Fatal("bad level must fail")
	}
	badSRGB := Instance{Name: "core", NET: "49.0001.1920.6800.1001.00", Level: Level12, SRGBStart: 20000, SRGBEnd: 10000}
	if err := badSRGB.Valid(); err == nil {
		t.Fatal("srgb end < start must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	c := Config{Instances: []Instance{
		{Name: "b", NET: "49.0002.0000.0000.0002.00", Level: Level2, Passive: []string{"lo"}, Redistribute: []string{"connected"}},
		{Name: "a", NET: "49.0001.0000.0000.0001.00", Level: Level12},
	}}
	first, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(c)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager()
	c1 := Config{Instances: []Instance{{Name: "a", NET: "49.0001.0000.0000.0001.00", Level: Level1}}}
	c2 := Config{Instances: []Instance{{Name: "a", NET: "49.0001.0000.0000.0001.00", Level: Level2}}}
	m.Set(c1)
	m.Set(c2)
	if err := m.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback(); err == nil {
		t.Fatal("double rollback must fail")
	}
}

func hashOf(s string) string {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	if h == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for h > 0 {
		i--
		buf[i] = byte('0' + h%10)
		h /= 10
	}
	return string(buf[i:])
}
