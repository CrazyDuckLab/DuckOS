// Package serviceunit exposes the exact shipped daemon unit for Agent ownership
// checks. The installer and compiled unit digest use one source file.
package serviceunit

import _ "embed"

//go:embed duck-dnsmasq.service
var dnsMasqUnit string

func DNSMasq() string { return dnsMasqUnit }

//go:embed duck-network-recover.service
var networkRecoveryUnit string

func NetworkRecovery() string { return networkRecoveryUnit }
