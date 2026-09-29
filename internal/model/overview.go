// Package model defines observed state, not desired configuration or mutations.
package model

import "time"

const SchemaVersion = "1"
const MaxResponseBytes = 2 << 20

func ValidEnvironment(v string) bool { return v == "dev" || v == "test" || v == "production" }

type Overview struct {
	SchemaVersion string      `json:"schemaVersion"`
	ObservedAt    time.Time   `json:"observedAt"`
	Environment   string      `json:"environment"`
	System        System      `json:"system"`
	CPU           CPU         `json:"cpu"`
	Memory        Memory      `json:"memory"`
	Storage       []Storage   `json:"storage"`
	Interfaces    []Interface `json:"interfaces"`
	Routes        []Route     `json:"routes"`
	DNS           DNS         `json:"dns"`
	Services      []Service   `json:"services"`
	NFTables      NFTables    `json:"nftables"`
	Issues        []Issue     `json:"issues"`
}
type System struct {
	Hostname      string  `json:"hostname"`
	Kernel        string  `json:"kernel"`
	OS            string  `json:"os"`
	Architecture  string  `json:"architecture"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
}
type CPU struct {
	LogicalCores int       `json:"logicalCores"`
	UsagePercent *float64  `json:"usagePercent"`
	LoadAverage  []float64 `json:"loadAverage"`
}
type Memory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
}
type Storage struct {
	Mountpoint     string `json:"mountpoint"`
	Filesystem     string `json:"filesystem"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
}
type Interface struct {
	Name      string    `json:"name"`
	Index     int       `json:"index"`
	MTU       int       `json:"mtu"`
	MAC       string    `json:"mac"`
	State     string    `json:"state"`
	Addresses []Address `json:"addresses"`
}
type Address struct {
	Address      string `json:"address"`
	Family       string `json:"family"`
	PrefixLength int    `json:"prefixLength"`
}
type Route struct {
	Family      string `json:"family"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Device      string `json:"device"`
	Protocol    string `json:"protocol"`
	Scope       string `json:"scope"`
	Table       string `json:"table"`
	Metric      uint64 `json:"metric"`
}
type DNS struct {
	Nameservers   []string `json:"nameservers"`
	SearchDomains []string `json:"searchDomains"`
	Source        string   `json:"source"`
}
type Service struct {
	Name        string `json:"name"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
	SubState    string `json:"subState"`
}
type NFTables struct {
	Status string `json:"status"`
	Tables int    `json:"tables"`
	Chains int    `json:"chains"`
	Rules  int    `json:"rules"`
}
type Issue struct {
	Component string `json:"component"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}
