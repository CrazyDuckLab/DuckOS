package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/netip"
	"reflect"
	"time"
)

// DecodeOverview rejects incompatible, partial or unbounded IPC responses before
// they can enter the Engine cache. Every JSON field in v1 is required.
func DecodeOverview(data []byte, environment string, now time.Time) (Overview, error) {
	var o Overview
	if len(data) > MaxResponseBytes {
		return o, errors.New("response limit")
	}
	if err := requiredFields(data, reflect.TypeOf(o)); err != nil {
		return o, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&o); err != nil {
		return o, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return o, errors.New("trailing response data")
	}
	if o.SchemaVersion != SchemaVersion || !ValidEnvironment(o.Environment) || o.Environment != environment || o.ObservedAt.IsZero() || o.ObservedAt.After(now.Add(5*time.Second)) || o.ObservedAt.Before(now.Add(-30*time.Second)) {
		return o, errors.New("incompatible or stale observation")
	}
	if o.CPU.LogicalCores < 0 || len(o.CPU.LoadAverage) != 0 && len(o.CPU.LoadAverage) != 3 || o.Memory.AvailableBytes > o.Memory.TotalBytes || o.Memory.UsedBytes > o.Memory.TotalBytes {
		return o, errors.New("invalid resource counters")
	}
	if p := o.CPU.UsagePercent; p != nil && (math.IsNaN(*p) || *p < 0 || *p > 100) {
		return o, errors.New("invalid CPU usage")
	}
	if o.System.UptimeSeconds < 0 || len(o.Interfaces) > 256 || len(o.Routes) > 8192 || len(o.Services) > 64 || len(o.Storage) > 256 || len(o.Issues) > 128 || len(o.DNS.Nameservers) > 64 || len(o.DNS.SearchDomains) > 64 {
		return o, errors.New("invalid observation bounds")
	}
	if o.NFTables.Status != "available" && o.NFTables.Status != "unavailable" || o.NFTables.Tables < 0 || o.NFTables.Chains < 0 || o.NFTables.Rules < 0 {
		return o, errors.New("invalid nftables state")
	}
	for _, value := range o.CPU.LoadAverage {
		if value < 0 {
			return o, errors.New("invalid load")
		}
	}
	for _, row := range o.Storage {
		if row.AvailableBytes > row.TotalBytes || row.UsedBytes > row.TotalBytes {
			return o, errors.New("invalid storage")
		}
	}
	for _, row := range o.Routes {
		if row.Family != "ipv4" && row.Family != "ipv6" {
			return o, errors.New("invalid route family")
		}
	}
	for _, item := range o.Interfaces {
		if item.Index < 0 || item.MTU < 0 || len(item.Addresses) > 1024 {
			return o, errors.New("invalid interface bounds")
		}
		for _, address := range item.Addresses {
			ip, err := netip.ParseAddr(address.Address)
			if err != nil || (ip.Is4() && address.Family != "ipv4") || (ip.Is6() && address.Family != "ipv6") {
				return o, errors.New("invalid address value")
			}
			max := 128
			if address.Family == "ipv4" {
				max = 32
			} else if address.Family != "ipv6" {
				return o, errors.New("invalid address family")
			}
			if address.PrefixLength < 0 || address.PrefixLength > max {
				return o, errors.New("invalid address prefix")
			}
		}
	}
	return o, nil
}

func requiredFields(data []byte, typ reflect.Type) error {
	if typ == reflect.TypeOf(time.Time{}) {
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil || object == nil {
			return errors.New("required object absent")
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			raw, ok := object[field.Tag.Get("json")]
			if !ok {
				return errors.New("required field absent")
			}
			if err := requiredFields(raw, field.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil || items == nil {
			return errors.New("required array absent")
		}
		for _, raw := range items {
			if err := requiredFields(raw, typ.Elem()); err != nil {
				return err
			}
		}
	case reflect.Pointer: // CPU usage is explicitly nullable.
	default:
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return errors.New("required scalar absent")
		}
	}
	return nil
}
