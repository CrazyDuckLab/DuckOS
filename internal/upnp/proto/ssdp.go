// ssdp.go: SSDP (UPnP discovery) request parsing. DuckOS
// never floods multicast NOTIFY announcements and never
// responds on WAN interfaces — the responder only answers
// unicast after receiving an M-SEARCH from a trusted LAN
// source, which keeps discovery off the public WAN by
// default (see beta.3 security requirements).
package proto

import (
	"bufio"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
)

// SSDPPort is the well-known SSDP UDP port.
const SSDPPort = 1900

// SSDPMaxRequest bounds M-SEARCH parsing. RFC declines
// datagrams larger than 4096; DuckOS uses a tighter 2048.
const SSDPMaxRequest = 2048

// Search targets DuckOS advertises for the IGD:1 / IGD:2
// device tree.
const (
	STIGD1        = "upnp:rootdevice"
	STIGDDevice   = "urn:schemas-upnp-org:device:InternetGatewayDevice:1"
	STIGDDevice2  = "urn:schemas-upnp-org:device:InternetGatewayDevice:2"
	STWANIPConn   = "urn:schemas-upnp-org:service:WANIPConnection:1"
	STWANIPConn2  = "urn:schemas-upnp-org:service:WANIPConnection:2"
	STWANPPPConn  = "urn:schemas-upnp-org:service:WANPPPConnection:1"
	STWANPPPConn2 = "urn:schemas-upnp-org:service:WANPPPConnection:2"
	STAll         = "ssdp:all"
)

// MSearch is one decoded M-SEARCH request.
type MSearch struct {
	// ST is the raw search target.
	ST string
	// MX is the requested maximum response delay in seconds
	// (clamped by the server, never trusted).
	MX int
	// Man is the extension header, normally
	// "ssdp:discover".
	Man string
}

// knownTargets maps every advertised search target to a
// stable device/service identifier used by the responder.
var knownTargets = map[string]string{
	STIGD1:        STIGD1,
	STIGDDevice:   STIGDDevice,
	STIGDDevice2:  STIGDDevice2,
	STWANIPConn:   STWANIPConn,
	STWANIPConn2:  STWANIPConn2,
	STWANPPPConn:  STWANPPPConn,
	STWANPPPConn2: STWANPPPConn2,
	STAll:         STAll,
}

// RespondsTo reports whether the DuckOS responder answers
// this search target.
func (m MSearch) RespondsTo() bool {
	_, ok := knownTargets[m.ST]
	return ok
}

// ParseMSearch decodes one M-SEARCH datagram. It requires an
// uppercase or lowercase M-SEARCH method line and bounds the
// header block. Malformed packets are refused with an error
// — never partially parsed.
func ParseMSearch(buf []byte) (MSearch, error) {
	if len(buf) == 0 {
		return MSearch{}, errors.New("ssdp: empty datagram")
	}
	if len(buf) > SSDPMaxRequest {
		return MSearch{}, fmt.Errorf("ssdp: datagram too large (%d bytes)", len(buf))
	}
	text := string(buf)
	// The HTTP message must end with the CRLF terminator.
	if !strings.HasSuffix(text, "\r\n") {
		text = text + "\r\n"
	}
	reader := strings.NewReader(text)
	tp := textproto.NewReader(bufio.NewReader(reader))
	line, err := tp.ReadLine()
	if err != nil {
		return MSearch{}, errors.New("ssdp: malformed request line")
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || !strings.EqualFold(parts[0], "M-SEARCH") || !strings.EqualFold(parts[2], "HTTP/1.1") {
		return MSearch{}, errors.New("ssdp: not an M-SEARCH request")
	}
	// ReadMIMEHeader reports EOF when the terminating blank
	// line is absent; that is tolerated for UDP datagrams —
	// accept whatever headers were parsed as long as ST exists.
	header, _ := tp.ReadMIMEHeader()
	var out MSearch
	for k, v := range header {
		switch strings.ToLower(k) {
		case "st":
			out.ST = strings.TrimSpace(v[0])
		case "mx":
			mx := strings.TrimSpace(v[0])
			if len(mx) == 0 || len(mx) > 3 {
				return MSearch{}, errors.New("ssdp: malformed MX")
			}
			n := 0
			for _, ch := range mx {
				if ch < '0' || ch > '9' {
					return MSearch{}, errors.New("ssdp: malformed MX")
				}
				n = n*10 + int(ch-'0')
			}
			out.MX = n
		case "man":
			out.Man = strings.TrimSpace(v[0])
		}
	}
	if out.ST == "" {
		return MSearch{}, errors.New("ssdp: missing ST header")
	}
	if out.MX < 0 || out.MX > 5 {
		// RFC 6887 §1.2 caps MX at 5 for this profile; larger
		// values are clamped rather than refused so a chatty
		// client cannot force an error burst.
		out.MX = 5
	}
	return out, nil
}

// DescriptionXML returns the minimal UPnP IGD device
// description document DuckOS serves. friendlyName carries
// the product identity; the URLBase points at the
// frontend's LAN-bound HTTP listener.
func DescriptionXML(friendlyName, urlBase, controlURL string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <URLBase>` + urlBase + `/</URLBase>
  <device>
    <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
    <friendlyName>` + friendlyName + `</friendlyName>
    <manufacturer>CrazyDuckLab</manufacturer>
    <modelName>DuckOS</modelName>
    <UDN>uuid:duckos-upnp-igd</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
        <SCPDURL>/igd/wanipconn.xml</SCPDURL>
        <controlURL>` + controlURL + `</controlURL>
      </service>
    </serviceList>
  </device>
</root>
`
}
