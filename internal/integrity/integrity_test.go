package integrity

import "testing"

func TestSecureBootValid(t *testing.T) {
	for _, s := range []SecureBootState{SecureBootSupported, SecureBootActive, SecureBootDegraded, SecureBootUnsupported} {
		if !s.Valid() {
			t.Fatalf("%s must be valid", s)
		}
	}
}

func TestIntegrityStore(t *testing.T) {
	s := NewIntegrityStore()
	s.Set(Integrity{SecureBoot: SecureBootActive, ReleaseHash: "abc", ObservedHash: "abc"})
	if !s.Get().Verified {
		t.Fatal("verified must be true")
	}
	s.Set(Integrity{SecureBoot: SecureBootActive, ReleaseHash: "abc", ObservedHash: "xyz"})
	if s.Get().Verified {
		t.Fatal("verified must be false")
	}
}

func TestSBOM(t *testing.T) {
	s := NewSBOMStore()
	s.Add(SBOMComponent{Name: "linux", Version: "6.1.0"})
	s.Add(SBOMComponent{Name: "frr", Version: "10.3"})
	if len(s.All()) != 2 {
		t.Fatal("all")
	}
}

func TestKernelFW(t *testing.T) {
	s := NewKernelFWStore()
	s.SetModule(KernelModule{Name: "e1000e", Required: true, Detected: true})
	s.SetModule(KernelModule{Name: "virtio_net", Required: true, Detected: false})
	missing := s.MissingRequired()
	if len(missing) != 1 || missing[0].Name != "virtio_net" {
		t.Fatalf("missing: %+v", missing)
	}
	s.SetFirmware(Firmware{Device: "eth0", Supported: true})
	if len(s.FirmwareAll()) != 1 {
		t.Fatal("firmware")
	}
}

func TestHWHealth(t *testing.T) {
	h := NewHWHealth()
	h.ObserveTemp(Temp{Sensor: "cpu0", C: 45.0})
	h.ObserveFan(Fan{Sensor: "fan1", RPM: 3000})
	h.SetSMART(SMART{Device: "sda", Health: "ok", TempC: 35})
	if len(h.Temps()) != 1 || len(h.Fans()) != 1 || len(h.SMARTAll()) != 1 {
		t.Fatal("sizes")
	}
}

func TestStorage(t *testing.T) {
	s := NewStorageStore()
	s.SetDisk(Disk{Path: "/dev/sda", SizeBytes: 1 << 40, FSType: "ext4", MountPoint: "/"})
	s.SetRAID(RAID{Name: "md0", Level: "raid1", State: "ok"})
	if len(s.Disks()) != 1 || len(s.RAIDs()) != 1 {
		t.Fatal("storage")
	}
}
