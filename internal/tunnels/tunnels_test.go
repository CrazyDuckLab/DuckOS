// tunnels/tunnels_test.go pins the V1
// tunnel model: IPsec, VXLAN/EVPN, MPLS.
package tunnels

import "testing"

func TestIPSecConfigValid(t *testing.T) {
	good := IPSecConfig{IKEVersion: 2, AuthMethod: "psk", PSKRef: "vault-ref", LocalNetworks: []string{"10.0.0.0/8"}, RemoteNetworks: []string{"192.168.0.0/16"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	cert := IPSecConfig{IKEVersion: 2, AuthMethod: "cert", CertRef: "cert-ref", LocalNetworks: []string{"10.0.0.0/8"}, RemoteNetworks: []string{"192.168.0.0/16"}}
	if err := cert.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := IPSecConfig{IKEVersion: 3, AuthMethod: "psk"}
	if err := bad.Valid(); err == nil {
		t.Fatal("IKE v3 must fail")
	}
	pskNoRef := IPSecConfig{IKEVersion: 2, AuthMethod: "psk", LocalNetworks: []string{"10.0.0.0/8"}, RemoteNetworks: []string{"192.168.0.0/16"}}
	if err := pskNoRef.Valid(); err == nil {
		t.Fatal("PSK without ref must fail")
	}
}

func TestVXLANConfigValid(t *testing.T) {
	good := VXLANConfig{VNI: 100, Local: "10.0.0.1", Group: "239.1.1.1", Dev: "eth0", Port: 4789, Bridge: "br-vx", Learning: true, EVPNEnabled: true, EVPNVNI: 100}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noGroup := VXLANConfig{VNI: 100, Local: "10.0.0.1", Dev: "eth0"}
	if err := noGroup.Valid(); err == nil {
		t.Fatal("VXLAN needs group or remote")
	}
	evpnNoVNI := VXLANConfig{VNI: 100, Local: "10.0.0.1", Group: "239.1.1.1", Dev: "eth0", EVPNEnabled: true}
	if err := evpnNoVNI.Valid(); err == nil {
		t.Fatal("EVPNEnabled requires EVPNVNI")
	}
}

func TestMPLSConfigValid(t *testing.T) {
	good := MPLSConfig{LabelMin: 100, LabelMax: 1000, L3VPNVRFs: []string{"vrf-cust"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := MPLSConfig{LabelMin: 2000, LabelMax: 1000}
	if err := bad.Valid(); err == nil {
		t.Fatal("labelMax < labelMin must fail")
	}
}

func TestTunnelValid(t *testing.T) {
	ipsec := Tunnel{ID: "ipsec-1", Kind: KindIPsecSite, Name: "site-a", State: StateUp, Local: "10.0.0.1", Remote: "10.0.0.2", IPSec: &IPSecConfig{IKEVersion: 2, AuthMethod: "psk", PSKRef: "v", LocalNetworks: []string{"10.0.0.0/8"}, RemoteNetworks: []string{"192.168.0.0/16"}}}
	if err := ipsec.Valid(); err != nil {
		t.Fatal(err)
	}
	vxlan := Tunnel{ID: "vx-1", Kind: KindVXLAN, Name: "vx1", State: StateUp, VXLAN: &VXLANConfig{VNI: 100, Local: "10.0.0.1", Group: "239.1.1.1", Dev: "eth0"}}
	if err := vxlan.Valid(); err != nil {
		t.Fatal(err)
	}
	ipsecNoCfg := Tunnel{ID: "ipsec-1", Kind: KindIPsecSite, Name: "site-a", State: StateUp}
	if err := ipsecNoCfg.Valid(); err == nil {
		t.Fatal("IPsec without cfg must fail")
	}
}

func TestManagerCRUDAndCapability(t *testing.T) {
	m := NewManager()
	_ = m.Set(Tunnel{ID: "t-1", Kind: KindGRE, Name: "g", State: StateUp})
	_ = m.Set(Tunnel{ID: "t-2", Kind: KindVXLAN, Name: "v", State: StateUp, VXLAN: &VXLANConfig{VNI: 1, Local: "1.1.1.1", Group: "239.0.0.1", Dev: "eth0"}})
	if len(m.List()) != 2 {
		t.Fatal("List count")
	}
	if !m.HasCapability(KindVXLAN) {
		t.Fatal("HasCapability VXLAN must be true")
	}
	if m.HasCapability(KindMPLS) {
		t.Fatal("HasCapability MPLS must be false")
	}
	m.Delete("t-1")
	if _, ok := m.Get("t-1"); ok {
		t.Fatal("Get after Delete must miss")
	}
}
