// vyos_test.go pins the V1 VyOS / Vyatta
// analyser.
package migrate

import "testing"

func TestVyOSAnalyseEthernet(t *testing.T) {
	src := []byte(`interfaces {
    ethernet eth0 {
        address 192.0.2.1/24
        description uplink
    }
}
`)
	items, _, err := analyseVyOS(src)
	if err != nil {
		t.Fatal(err)
	}
	recognized := 0
	for _, it := range items {
		if it.Status == StatusRecognized {
			recognized++
		}
	}
	if recognized < 2 {
		t.Fatalf("expected at least 2 recognized, got %d (%+v)", recognized, items)
	}
}

func TestVyOSAnalyseBGP(t *testing.T) {
	src := []byte(`protocols {
    bgp 64512 {
        neighbor 10.0.0.1 {
            remote-as 64513
        }
        neighbor 10.0.0.2 {
            remote-as 64513
        }
        neighbor 10.0.0.3 {
            remote-as 64514
        }
    }
}
`)
	items, _, err := analyseVyOS(src)
	if err != nil {
		t.Fatal(err)
	}
	recognized := 0
	for _, it := range items {
		if it.Status == StatusRecognized {
			recognized++
		}
	}
	if recognized < 3 {
		t.Fatalf("expected at least 3 recognized, got %d (%+v)", recognized, items)
	}
}

func TestVyOSAnalyseUnknownSection(t *testing.T) {
	src := []byte(`unknown-section {
    something 1
}
`)
	items, _, err := analyseVyOS(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items: %d", len(items))
	}
	if items[0].Status != StatusUnsupported {
		t.Fatalf("expected unsupported, got %s", items[0].Status)
	}
}

func TestVyOSAnalyseConvertible(t *testing.T) {
	src := []byte(`firewall {
    name mychain {
        default-action drop
    }
}
`)
	items, _, err := analyseVyOS(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected items")
	}
	convertible := 0
	for _, it := range items {
		if it.Status == StatusConvertible {
			convertible++
		}
	}
	if convertible < 1 {
		t.Fatalf("expected at least 1 convertible, got %d", convertible)
	}
}

func TestVyOSUnbalancedBrace(t *testing.T) {
	src := []byte("}")
	items, _, err := analyseVyOS(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Intent != "unbalanced" {
		t.Fatalf("expected unbalanced, got %+v", items)
	}
}

func TestDetectFormatVyOS(t *testing.T) {
	src := []byte(`interfaces {
    ethernet eth0 {
        address 192.0.2.1/24
    }
}
`)
	format, conf := DetectFormat(src)
	if format != FormatVyOS {
		t.Fatalf("format: %s", format)
	}
	if conf == ConfidenceLow {
		t.Fatalf("confidence: %s", conf)
	}
}
