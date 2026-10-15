// Package center / render.go: content renderer bridging the
// durable catalogs to the subscription format engine. A subID is
// either a subscription group id or an xray subscription id
// (profile-based). Resolved credentials come from the caller-supplied
// resolver (the vault-backed resolver on the agent); no secret bytes
// are logged, and REALITY private keys never enter the output —
// only the public key and short ids do.
package center

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/subscription"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// CatalogSource is the read surface the renderer needs.
type CatalogSource interface {
	ListServers(ctx context.Context) ([]xray.Server, error)
	ListUsers(ctx context.Context, serverID string) ([]xray.User, error)
}

// RendererOptions wire the renderer.
type RendererOptions struct {
	Catalog CatalogSource
	Groups  *subscription.GroupStore
	// Resolver resolves vault references (xray-credential,
	// reality-shortid). Required for vault-bound objects.
	Resolver   xray.Resolver
	CtxTimeout time.Duration
}

// BuildRenderer returns the Render func consumable by the Handler
// and the delivery handler. kind is "group" or "subscription".
// Errors are the caller's to map onto delivery 404s; the message
// never carries secret material.
func BuildRenderer(o RendererOptions) func(kind, subID string, format subscription.Format) ([]byte, error) {
	return func(kind, subID string, format subscription.Format) ([]byte, error) {
		if o.Catalog == nil {
			return nil, errors.New("render: catalog is unavailable")
		}
		switch kind {
		case "group":
			if o.Groups == nil {
				return nil, errors.New("render: groups are unavailable")
			}
			if _, ok := o.Groups.Get(subID); !ok {
				return nil, errors.New("render: group not found")
			}
			timeout := o.CtxTimeout
			if timeout <= 0 {
				timeout = 5 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return renderGroup(o, ctx, subID, format)
		case "subscription":
			proxies, err := renderProfiles(o, subID)
			if err != nil {
				return nil, err
			}
			return subscription.Render(format, proxies)
		default:
			return nil, errors.New("render: unknown kind")
		}
	}
}

func renderGroup(o RendererOptions, ctx context.Context, groupID string, format subscription.Format) ([]byte, error) {
	if o.Groups == nil {
		return nil, errors.New("render: groups are unavailable")
	}
	now := time.Now()
	members, err := o.Groups.GroupProxies(groupID, now, func(m subscription.GroupMember) (subscription.Proxy, error) {
		return memberProxy(o, ctx, m.ServerID, m.UserID, m.DisplayName)
	})
	if err != nil {
		return nil, err
	}
	return subscription.Render(format, members)
}

func renderProfiles(o RendererOptions, subID string) ([]subscription.Proxy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The xray subscription record lives in the durable catalog
	// through the API layer; the renderer receives subscriptions
	// through the same FileCatalog listers.
	src, ok := o.Catalog.(interface {
		GetSubscription(ctx context.Context, id string) (xray.Subscription, error)
	})
	if !ok {
		return nil, errors.New("render: subscriptions are unavailable")
	}
	sub, err := src.GetSubscription(ctx, subID)
	if err != nil {
		return nil, err
	}
	if !sub.Enabled || sub.IsExpired(time.Now()) {
		return nil, errors.New("render: subscription is disabled or expired")
	}
	var out []subscription.Proxy
	for _, pid := range sub.ProfileIDs {
		profile, err := getProfile(o.Catalog, ctx, pid)
		if err != nil {
			continue
		}
		if !profile.Enabled || profile.IsExpired(time.Now()) {
			continue
		}
		proxy, err := memberProxy(o, ctx, "", profile.UserID, profile.Name)
		if err != nil {
			continue
		}
		out = append(out, proxy)
	}
	if len(out) == 0 {
		return nil, errors.New("render: no usable profiles")
	}
	return out, nil
}

// memberProxy resolves one (server, user) pair into a delivery
// proxy. The display name is the client-facing label.
func memberProxy(o RendererOptions, ctx context.Context, serverID, userID, displayName string) (subscription.Proxy, error) {
	servers, err := o.Catalog.ListServers(ctx)
	if err != nil {
		return subscription.Proxy{}, err
	}
	var user *xray.User
	var server *xray.Server
	for i := range servers {
		if serverID != "" && servers[i].ID != serverID {
			continue
		}
		users, err := o.Catalog.ListUsers(ctx, servers[i].ID)
		if err != nil {
			continue
		}
		for j := range users {
			if users[j].ID == userID {
				server, user = &servers[i], &users[j]
				break
			}
		}
		if server != nil {
			break
		}
	}
	if server == nil || user == nil {
		return subscription.Proxy{}, fmt.Errorf("render: object %s/%s not found", serverID, userID)
	}
	if !server.Enabled || !user.Enabled || user.IsExpired(time.Now()) {
		return subscription.Proxy{}, errors.New("render: object is disabled or expired")
	}
	proxy := subscription.Proxy{
		Name: displayName,
		Port: server.Port,
		Flow: user.Flow,
		UDP:  true,
	}
	switch server.Protocol {
	case xray.ProtocolVLESS:
		proxy.Protocol = "vless"
		proxy.UUID = user.UUID
	case xray.ProtocolVMess:
		proxy.Protocol = "vmess"
		proxy.UUID = user.UUID
	case xray.ProtocolTrojan:
		proxy.Protocol = "trojan"
		proxy.Password = user.Password
	case xray.ProtocolShadowsocks:
		proxy.Protocol = "ss"
		proxy.Password = user.Password
		proxy.Cipher = server.SSMethod
	default:
		return subscription.Proxy{}, fmt.Errorf("render: protocol %q has no client format", server.Protocol)
	}
	proxy.Transport = string(server.Transport)
	proxy.Security = string(server.Security)
	if ts := server.TransportSettings; ts != nil {
		if ts.WS != nil {
			proxy.Path, proxy.HeaderHost = ts.WS.Path, ts.WS.Host
		}
		if ts.GRPC != nil {
			proxy.ServiceName = ts.GRPC.ServiceName
		}
		if ts.XHTTP != nil {
			proxy.Path, proxy.HeaderHost, proxy.Mode = ts.XHTTP.Path, ts.XHTTP.Host, ts.XHTTP.Mode
		}
		if ts.TCP != nil {
			proxy.HeaderType = ts.TCP.HeaderType
		}
	}
	proxy.SNI = firstSNI(*server)
	proxy.Fingerprint = server.Fingerprint
	proxy.PublicKey = server.RealityPublicKey
	if host, _, err := splitListen(server.Listen); err == nil {
		proxy.Host = host
	}
	// Resolve vault-bound credentials.
	if o.Resolver != nil {
		if user.Credential != nil {
			secret, err := o.Resolver.Resolve(ctx, *user.Credential)
			if err != nil {
				return subscription.Proxy{}, errors.New("render: credential is unavailable")
			}
			setCredential(&proxy, trimSecret(secret))
		}
		if server.Security == xray.SecurityReality && server.ShortIDRef != nil {
			sid, err := o.Resolver.Resolve(ctx, *server.ShortIDRef)
			if err == nil {
				proxy.ShortID = hexOf(sid)
			}
		}
	}
	if proxy.UUID == "" && proxy.Password == "" {
		return subscription.Proxy{}, errors.New("render: credential is unavailable")
	}
	if server.Security == xray.SecurityReality && (proxy.PublicKey == "" || proxy.ShortID == "") {
		return subscription.Proxy{}, errors.New("render: REALITY parameters are incomplete")
	}
	return proxy, nil
}

func setCredential(p *subscription.Proxy, secret string) {
	switch p.Protocol {
	case "vless", "vmess":
		p.UUID = secret
	default:
		p.Password = secret
	}
}

func getProfile(src CatalogSource, ctx context.Context, profileID string) (xray.Profile, error) {
	if getter, ok := src.(interface {
		GetProfile(ctx context.Context, id string) (xray.Profile, error)
	}); ok {
		return getter.GetProfile(ctx, profileID)
	}
	return xray.Profile{}, errors.New("render: profile lookup is unavailable")
}

func firstSNI(s xray.Server) string {
	if len(s.RealitySNI) > 0 {
		return s.RealitySNI[0]
	}
	return ""
}
