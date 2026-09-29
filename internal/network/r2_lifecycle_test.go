package network

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderEmitsBridgeNetdev(t *testing.T) {
	c, _ := Decode([]byte(`{
        "version": 1,
        "interfaces": [
            {"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},
            {"id":"child","name":"testport0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":false,"addresses":[],"parent":"guest"},
            {"id":"guest","name":"testguest0","kind":"bridge","role":"lan","mtu":1500,"dhcpClient":"none","management":false,"addresses":["203.0.113.1/24"]}
        ]
    }`))
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	netdev, ok := plan.Networkd["10-duckos-guest.netdev"]
	if !ok {
		t.Fatal("bridge netdev was not rendered")
	}
	if !containsAll(netdev, "Kind=bridge", "Bind=testport0") {
		t.Fatal("bridge netdev missing required fields", netdev)
	}
}

func TestRenderEmitsDHCPClientAndAcceptRA(t *testing.T) {
	c, _ := Decode([]byte(`{
        "version": 1,
        "interfaces": [
            {"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},
            {"id":"uplink","name":"testwan0","kind":"physical","role":"wan","mtu":1500,"dhcpClient":"ipv4","management":false,"addresses":[],"acceptRA":true}
        ]
    }`))
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	body := plan.Networkd["10-duckos-uplink.network"]
	if !containsAll(body, "DHCP=ipv4", "IPv6AcceptRA=yes") {
		t.Fatal("missing DHCP/RA fields", body)
	}
}

func TestRenderEmitsVLANAndParentReference(t *testing.T) {
	c, _ := Decode([]byte(`{
        "version": 1,
        "interfaces": [
            {"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},
            {"id":"uplink","name":"testwan0","kind":"physical","role":"wan","mtu":1500,"dhcpClient":"none","management":false,"addresses":[]},
            {"id":"v10","name":"vlan10-uplink","kind":"vlan","role":"lan","mtu":1500,"dhcpClient":"none","management":false,"addresses":["203.0.113.10/24"],"parent":"uplink","vlan":10}
        ]
    }`))
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	netdev, ok := plan.Networkd["10-duckos-v10.netdev"]
	if !ok {
		t.Fatal("vlan netdev was not rendered")
	}
	if !containsAll(netdev, "Kind=vlan", "Id=10") {
		t.Fatal("vlan netdev missing required fields", netdev)
	}
	// The VLAN must be wired under its parent in the parent's .network file.
	uplink := plan.Networkd["10-duckos-uplink.network"]
	if !contains(uplink, "VLAN=vlan10-uplink") {
		t.Fatal("uplink network file did not reference the VLAN child")
	}
}

func TestRenderDHCPServiceRequiresLANListener(t *testing.T) {
	c, _ := Decode([]byte(`{
        "version": 1,
        "dns": {"upstreams":["192.0.2.53"]},
        "interfaces": [
            {"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},
            {"id":"uplink","name":"testwan0","kind":"physical","role":"wan","mtu":1500,"dhcpClient":"none","management":false,"addresses":[]}
        ]
    }`))
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := Render(c)
	if err != nil || plan.DNSMasq == "" {
		t.Fatal("DNS service was not rendered with a LAN listener", err)
	}
}

func TestManagedRoutingPairAcceptsDHCPClient(t *testing.T) {
	before := []byte(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},{"id":"uplink","name":"testwan0","kind":"physical","role":"wan","mtu":1500,"dhcpClient":"ipv4","management":false,"addresses":[],"acceptRA":true}]}`)
	next := []byte(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"]},{"id":"uplink","name":"testwan0","kind":"physical","role":"wan","mtu":1500,"dhcpClient":"dual","management":false,"addresses":[],"acceptRA":true}]}`)
	if _, _, err := managedRoutingPair(before, next); err != nil {
		t.Fatal("dynamic link change was rejected", err)
	}
}

func TestManagedRoutingPairRejectsInvalidDHCPRange(t *testing.T) {
	raw := []byte(`{"version":1,"interfaces":[{"id":"inside","name":"testlan0","kind":"physical","role":"lan","mtu":1500,"dhcpClient":"none","management":true,"addresses":["198.51.100.1/24"],"dhcpServer":{"start":"not-an-ip","end":"198.51.100.200","leaseSeconds":3600}}]}`)
	if _, err := Decode(raw); err == nil {
		t.Fatal("invalid DHCP range was accepted at decode time")
	}
}

func TestManagedRoutingAdapterAttachesDNSArtifacts(t *testing.T) {
	dir := t.TempDir()
	dnsDir := filepath.Join(dir, "dns")
	if err := os.MkdirAll(dnsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	d, err := OpenDNSArtifacts(dnsDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(nil, map[string]string{"dnsmasq.conf": "port=53\n"}); err != nil {
		t.Fatal(err)
	}
	d.Close()
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}
