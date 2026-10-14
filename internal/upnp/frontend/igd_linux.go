//go:build linux

// igd_linux.go: the UPnP IGD control endpoint — a minimal
// HTTP server bound to a single trusted LAN address. It
// serves the device description XML and the WANIPConnection
// SOAP actions needed by common clients:
//
//	GetExternalIPAddress
//	AddPortMapping
//	DeletePortMapping
//	GetGenericPortMappingEntry (mapping table index walk)
//
// The listener binds ONE address explicitly (never the
// wildcard) and is reachable only from the trusted LAN. The
// SOAP body is size-bounded and parsed with encoding/xml;
// control timeouts keep stuck clients from pinning the
// server. All mapping mutations go through the Server's
// protocol path with source "upnp-igd:<client>" so ownership
// and audit stay identical to NAT-PMP/PCP.
package frontend

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
	"github.com/CrazyDuckLab/DuckOS/internal/upnp/proto"
)

// igdServer is one HTTP listener for one trusted address.
type igdServer struct {
	srv    *http.Server
	cancel context.CancelFunc
	// mappingIDFromAction records ids created here so
	// DeletePortMapping finds them deterministically.
	mu    sync.Mutex
	ids   map[string]string // clientIP:extPort:transport -> mapping id
	local netip.Addr
}

func newIGDServer(ctx context.Context, f *Server, networkRef string, addr netip.Addr) (*igdServer, error) {
	g := &igdServer{ids: map[string]string{}, local: addr}
	mux := http.NewServeMux()
	mux.HandleFunc("/igd/description.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, proto.DescriptionXML("DuckOS Gateway", "http://"+addr.String()+":"+strconv.Itoa(IGDControlPort), "/igd/control"))
	})
	mux.HandleFunc("/igd/control", func(w http.ResponseWriter, r *http.Request) {
		g.handleControl(w, r, f, networkRef)
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	g.srv = srv
	ln, err := net.Listen("tcp4", net.JoinHostPort(addr.String(), strconv.Itoa(IGDControlPort)))
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	go func() {
		<-cctx.Done()
		_ = srv.Close()
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("frontend: igd server on %s stopped: %v", addr, err)
		}
	}()
	return g, nil
}

func (g *igdServer) close() {
	g.cancel()
}

// igdAction is one decoded SOAP action envelope.
type igdAction struct {
	XMLName xml.Name
	Action  string
	// AddPortMapping fields.
	NewRemoteHost     string `xml:"NewRemoteHost"`
	NewExternalPort   int    `xml:"NewExternalPort"`
	NewProtocol       string `xml:"NewProtocol"`
	NewInternalPort   int    `xml:"NewInternalPort"`
	NewInternalClient string `xml:"NewInternalClient"`
	NewEnabled        int    `xml:"NewEnabled"`
	// DeletePortMapping fields (same External/Protocol).
	// GetGenericPortMappingEntry:
	NewPortMappingIndex int `xml:"NewPortMappingIndex"`
}

// soapFault renders a UPnP SOAP fault.
func soapFault(w http.ResponseWriter, code, description string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	body := fmt.Sprintf(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%s</errorCode><errorDescription>%s</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, code, description)
	_, _ = io.WriteString(w, body)
}

// soapOK renders an action result with optional out params.
func soapOK(w http.ResponseWriter, service string, action string, params string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:` + action + `Response xmlns:u="urn:schemas-upnp-org:service:` + service + `:1">`)
	b.WriteString(params)
	b.WriteString(`</u:` + action + `Response></s:Body></s:Envelope>`)
	_, _ = io.WriteString(w, b.String())
}

const maxSOAPBody = 16 << 10

// handleControl dispatches the SOAP control action.
func (g *igdServer) handleControl(w http.ResponseWriter, r *http.Request, f *Server, networkRef string) {
	clientIP := ""
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	} else {
		clientIP = host
	}
	action := soapAction(r.Header.Get("SOAPACTION"))
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSOAPBody+1))
	if err != nil || len(body) > maxSOAPBody {
		soapFault(w, "402", "Invalid Args")
		return
	}
	var req igdAction
	if len(body) > 0 {
		if err := xml.Unmarshal(body, &req); err != nil {
			soapFault(w, "402", "Invalid Args")
			return
		}
	}
	if action == "" {
		action = req.Action
	}
	switch action {
	case "GetExternalIPAddress":
		ext := f.Status().ExternalIPv4
		if ext == "" {
			ext = "0.0.0.0"
		}
		soapOK(w, "WANIPConnection", "GetExternalIPAddress", "<NewExternalIPAddress>"+ext+"</NewExternalIPAddress>")
	case "AddPortMapping":
		g.addPortMapping(w, f, networkRef, clientIP, &req)
	case "DeletePortMapping":
		g.deletePortMapping(w, f, clientIP, &req)
	case "GetGenericPortMappingEntry":
		g.genericEntry(w, f, req.NewPortMappingIndex)
	case "":
		soapFault(w, "401", "Invalid Action")
	default:
		soapFault(w, "401", "Invalid Action")
	}
}

// soapAction extracts the action name from the SOAPACTION
// header ("urn:...#AddPortMapping").
func soapAction(header string) string {
	if header == "" {
		return ""
	}
	header = strings.Trim(header, `"`)
	if i := strings.LastIndex(header, "#"); i >= 0 && i+1 < len(header) {
		return header[i+1:]
	}
	return ""
}

func (g *igdServer) addPortMapping(w http.ResponseWriter, f *Server, networkRef, clientIP string, req *igdAction) {
	transport := upnp.Transport(strings.ToLower(req.NewProtocol))
	if transport != upnp.TransportTCP && transport != upnp.TransportUDP {
		soapFault(w, "402", "Invalid Args (protocol)")
		return
	}
	if req.NewExternalPort < 1 || req.NewExternalPort > 65535 ||
		req.NewInternalPort < 1 || req.NewInternalPort > 65535 {
		soapFault(w, "402", "Invalid Args (ports)")
		return
	}
	// The internal host must be the requesting client. IGD
	// requests that open ports for third parties are refused.
	if req.NewInternalClient == "" || req.NewInternalClient != clientIP {
		soapFault(w, "718", "ConflictInMappingEntry")
		return
	}
	network := upnp.Network{NetworkRef: networkRef, Protocols: []upnp.Protocol{upnp.ProtoUPnP}, TTLSec: 0}
	mappings := f.core.sink.Mappings()
	extPort := req.NewExternalPort
	if !portFreeIGD(f, mappings, transport, extPort, clientIP, req.NewInternalPort) {
		soapFault(w, "718", "ConflictInMappingEntry")
		return
	}
	now := time.Now()
	lifetime := uint32(f.core.limits.MaxLifetimeSec)
	m := buildMapping(upnp.ProtoUPnP, transport, network, clientIP, req.NewInternalPort, extPort, lifetime, now)
	m.ID = renewalID(mappings, m.ID, m)
	if err := f.core.sink.AddMapping(m, "upnp-igd:"+clientIP); err != nil {
		soapFault(w, "501", "ActionFailed")
		return
	}
	g.mu.Lock()
	g.ids[clientIP+":"+strconv.Itoa(extPort)+":"+string(transport)] = m.ID
	g.mu.Unlock()
	f.core.observeSuccess()
	soapOK(w, "WANIPConnection", "AddPortMapping", "")
}

func (g *igdServer) deletePortMapping(w http.ResponseWriter, f *Server, clientIP string, req *igdAction) {
	transport := upnp.Transport(strings.ToLower(req.NewProtocol))
	if transport != upnp.TransportTCP && transport != upnp.TransportUDP {
		soapFault(w, "402", "Invalid Args (protocol)")
		return
	}
	g.mu.Lock()
	id, ok := g.ids[clientIP+":"+strconv.Itoa(req.NewExternalPort)+":"+string(transport)]
	if ok {
		delete(g.ids, clientIP+":"+strconv.Itoa(req.NewExternalPort)+":"+string(transport))
	}
	g.mu.Unlock()
	if !ok {
		// Not owned by this control endpoint; still idempotent.
		soapOK(w, "WANIPConnection", "DeletePortMapping", "")
		return
	}
	f.sinkDelete(id)
	soapOK(w, "WANIPConnection", "DeletePortMapping", "")
}

func (g *igdServer) genericEntry(w http.ResponseWriter, f *Server, index int) {
	mappings := f.core.sink.Mappings()
	if index < 0 || index >= len(mappings) {
		soapFault(w, "713", "SpecifiedArrayIndexInvalid")
		return
	}
	m := mappings[index]
	protoStr := string(m.Transport)
	if protoStr == "" {
		protoStr = "tcp"
	}
	params := fmt.Sprintf(
		"<NewRemoteHost></NewRemoteHost><NewExternalPort>%d</NewExternalPort><NewProtocol>%s</NewProtocol><NewInternalPort>%d</NewInternalPort><NewInternalClient>%s</NewInternalClient><NewEnabled>1</NewEnabled><NewPortMappingDescription>DuckOS %s</NewPortMappingDescription><NewLeaseDuration>%d</NewLeaseDuration>",
		m.ExternalPort, protoStr, m.InternalPort, m.ClientIP, string(m.Protocol),
		int(time.Until(m.ExpiresAt).Seconds()))
	soapOK(w, "WANIPConnection", "GetGenericPortMappingEntry", params)
}

// portFreeIGD mirrors the NAT-PMP/PCP conflict rule.
func portFreeIGD(f *Server, mappings []upnp.Mapping, transport upnp.Transport, extPort int, clientIP string, intPort int) bool {
	return portFree(mappings, transport, extPort, clientIP, intPort)
}

// sinkDelete is the MappingSink delete seam (revoke by id).
// The MappingSink interface only exposes Add/List; deletion
// flows through the same runtime the agent uses. To keep the
// IGD handler honest without widening the sink contract, the
// agent passes a Deleter when constructing the Server.
func (f *Server) sinkDelete(id string) {
	if d, ok := f.core.sink.(MappingDeleter); ok {
		d.RevokeMapping(id)
	}
}

// MappingDeleter is the optional revoke seam.
type MappingDeleter interface {
	RevokeMapping(id string)
}

var _ = bytes.Equal
