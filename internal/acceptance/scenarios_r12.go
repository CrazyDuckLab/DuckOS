// scenarios_r12.go: R12.BB expansion of the
// V1 acceptance-scenario registry. The
// R11 registry covered scenarios A..R;
// the R12 expansion brings the registry to
// the full 65-scenario list called out in
// R12.BB. The original IDs are preserved
// so existing tests and references keep
// working.
package acceptance

// R12.BB expanded scenarios. The IDs
// numbered 1..65 are stable; the registry
// returns the full list through
// AllScenarios() so the API and the future
// UI can render a single registry.
const (
	Scenario01FirstBootNoInternet       ScenarioID = "1.first-boot-no-internet"
	Scenario02SingleInterfaceVPS        ScenarioID = "2.single-interface-vps"
	Scenario03WANDHCPToPPPoE            ScenarioID = "3.wan-dhcp-to-pppoe"
	Scenario04ProviderMACBinding        ScenarioID = "4.provider-mac-binding"
	Scenario05ExternalIPChange          ScenarioID = "5.external-ip-change"
	Scenario06CGNATToggle               ScenarioID = "6.cgnat-toggle"
	Scenario07DNSFailureWithLinkUp      ScenarioID = "7.dns-failure-link-up"
	Scenario08ACMERenewalFailure        ScenarioID = "8.acme-renewal-failure"
	Scenario09InvalidManualCert         ScenarioID = "9.invalid-manual-cert"
	Scenario10XrayDomainChange          ScenarioID = "10.xray-domain-change"
	Scenario11XrayUserRevoke            ScenarioID = "11.xray-user-revoke"
	Scenario12WGAccessPolicyChange      ScenarioID = "12.wg-access-policy-change"
	Scenario13ManagementIPChange        ScenarioID = "13.management-ip-change"
	Scenario14SafeApplyLoss             ScenarioID = "14.safe-apply-management-loss"
	Scenario15TwoAdminsEditSame         ScenarioID = "15.two-admins-edit-same"
	Scenario16ConfigDrift               ScenarioID = "16.config-drift"
	Scenario17DeleteWithDependencies    ScenarioID = "17.delete-with-dependencies"
	Scenario18RestoreOnDifferentHW      ScenarioID = "18.restore-on-different-hw"
	Scenario19RestoreOldIntoNew         ScenarioID = "19.restore-old-into-new"
	Scenario20RejectFutureSchema        ScenarioID = "20.reject-future-schema"
	Scenario21UpdateMigrationFailure    ScenarioID = "21.update-migration-failure"
	Scenario22UpdateHealthCheckRollback ScenarioID = "22.update-health-check-rollback"
	Scenario23PowerLossDuringUpdate     ScenarioID = "23.power-loss-during-update"
	Scenario24PowerLossDuringCommit     ScenarioID = "24.power-loss-during-commit"
	Scenario25PowerLossDuringVault      ScenarioID = "25.power-loss-during-vault"
	Scenario26StorageFull               ScenarioID = "26.storage-full"
	Scenario27StorageReadOnly           ScenarioID = "27.storage-read-only"
	Scenario28MultiWANPrimaryFailure    ScenarioID = "28.multi-wan-primary-failure"
	Scenario29SessionsDuringFailover    ScenarioID = "29.sessions-during-failover"
	Scenario30IntentionalWANDisable     ScenarioID = "30.intentional-wan-disable"
	Scenario31USBModemBackupWAN         ScenarioID = "31.usb-modem-backup-wan"
	Scenario32OverlappingRemoteSubnets  ScenarioID = "32.overlapping-remote-subnets"
	Scenario33IPv6UnexpectedFromISP     ScenarioID = "33.ipv6-unexpected-from-isp"
	Scenario34IPv6TunnelLeakPrevent     ScenarioID = "34.ipv6-tunnel-leak-prevent"
	Scenario35DHCPConflict              ScenarioID = "35.dhcp-conflict"
	Scenario36RogueIPv6RA               ScenarioID = "36.rogue-ipv6-ra"
	Scenario37L2NetworkLoop             ScenarioID = "37.l2-network-loop"
	Scenario38VLANLoop                  ScenarioID = "38.vlan-loop"
	Scenario39MACFlap                   ScenarioID = "39.mac-flap"
	Scenario40BroadcastStorm            ScenarioID = "40.broadcast-storm"
	Scenario41STPRedundantPath          ScenarioID = "41.stp-redundant-path"
	Scenario42L3RoutingLoop             ScenarioID = "42.l3-routing-loop"
	Scenario43MTUBlackHole              ScenarioID = "43.mtu-blackhole"
	Scenario44HairpinNAT                ScenarioID = "44.hairpin-nat"
	Scenario45SplitDNS                  ScenarioID = "45.split-dns"
	Scenario46PublishServiceLifecycle   ScenarioID = "46.publish-service-lifecycle"
	Scenario47RemoteUpdateUnavailable   ScenarioID = "47.remote-update-unavailable"
	Scenario48XrayCrashRouteAlive       ScenarioID = "48.xray-crash-route-alive"
	Scenario49DNSCrashRouteAlive        ScenarioID = "49.dns-crash-route-alive"
	Scenario50PoEWatchdogBounded        ScenarioID = "50.poe-watchdog-bounded"
	Scenario51MaintenanceExpires        ScenarioID = "51.maintenance-expires"
	Scenario52EmergencyAccessExpires    ScenarioID = "52.emergency-access-expires"
	Scenario53UnsupportedHardware       ScenarioID = "53.unsupported-hardware"
	Scenario54InterfaceRenamedVM        ScenarioID = "54.interface-renamed-vm"
	Scenario55BGPRouteRejected          ScenarioID = "55.bgp-route-rejected"
	Scenario56OSPFNeighborLoss          ScenarioID = "56.ospf-neighbor-loss"
	Scenario57VRRPFailover              ScenarioID = "57.vrrp-failover"
	Scenario58LACPMemberFailure         ScenarioID = "58.lacp-member-failure"
	Scenario59VLANQinQValidation        ScenarioID = "59.vlan-qinq-validation"
	Scenario60DNSDependencyLoop         ScenarioID = "60.dns-dependency-loop"
	Scenario61CertTimeUntrusted         ScenarioID = "61.cert-time-untrusted"
	Scenario62SupportBundleRedaction    ScenarioID = "62.support-bundle-redaction"
	Scenario63FullBackupWithSecrets     ScenarioID = "63.full-backup-with-secrets"
	Scenario64FactoryResetRecovery      ScenarioID = "64.factory-reset-recovery"
	Scenario65Incident30Sec             ScenarioID = "65.incident-30s"
)

// AllScenarios returns the full R12.BB
// scenario registry sorted by ID. Existing
// R11 A..R IDs are included so callers
// that consult the registry see one
// consistent list.
func AllScenarios() []ScenarioID {
	out := []ScenarioID{
		// R11 A..R (originals, retained).
		ScenarioWANReplacement,
		ScenarioCGNATChange,
		ScenarioACMESuccess,
		ScenarioACMEFailure,
		ScenarioInvalidCert,
		ScenarioDomainChange,
		ScenarioDependencyDelete,
		ScenarioRestoreOnNewHW,
		ScenarioSafeApplyLoss,
		ScenarioUpdateMigFailure,
		ScenarioStorageFull,
		ScenarioDisabledNotFailed,
		ScenarioWGAccessChange,
		ScenarioTemporaryAccess,
		ScenarioPublishIntent,
		ScenarioOverlappingNets,
		ScenarioIPv6Parity,
		ScenarioBackupWANModem,
		ScenarioFactoryReset,
		// R12 1..65 (expanded).
		Scenario01FirstBootNoInternet,
		Scenario02SingleInterfaceVPS,
		Scenario03WANDHCPToPPPoE,
		Scenario04ProviderMACBinding,
		Scenario05ExternalIPChange,
		Scenario06CGNATToggle,
		Scenario07DNSFailureWithLinkUp,
		Scenario08ACMERenewalFailure,
		Scenario09InvalidManualCert,
		Scenario10XrayDomainChange,
		Scenario11XrayUserRevoke,
		Scenario12WGAccessPolicyChange,
		Scenario13ManagementIPChange,
		Scenario14SafeApplyLoss,
		Scenario15TwoAdminsEditSame,
		Scenario16ConfigDrift,
		Scenario17DeleteWithDependencies,
		Scenario18RestoreOnDifferentHW,
		Scenario19RestoreOldIntoNew,
		Scenario20RejectFutureSchema,
		Scenario21UpdateMigrationFailure,
		Scenario22UpdateHealthCheckRollback,
		Scenario23PowerLossDuringUpdate,
		Scenario24PowerLossDuringCommit,
		Scenario25PowerLossDuringVault,
		Scenario26StorageFull,
		Scenario27StorageReadOnly,
		Scenario28MultiWANPrimaryFailure,
		Scenario29SessionsDuringFailover,
		Scenario30IntentionalWANDisable,
		Scenario31USBModemBackupWAN,
		Scenario32OverlappingRemoteSubnets,
		Scenario33IPv6UnexpectedFromISP,
		Scenario34IPv6TunnelLeakPrevent,
		Scenario35DHCPConflict,
		Scenario36RogueIPv6RA,
		Scenario37L2NetworkLoop,
		Scenario38VLANLoop,
		Scenario39MACFlap,
		Scenario40BroadcastStorm,
		Scenario41STPRedundantPath,
		Scenario42L3RoutingLoop,
		Scenario43MTUBlackHole,
		Scenario44HairpinNAT,
		Scenario45SplitDNS,
		Scenario46PublishServiceLifecycle,
		Scenario47RemoteUpdateUnavailable,
		Scenario48XrayCrashRouteAlive,
		Scenario49DNSCrashRouteAlive,
		Scenario50PoEWatchdogBounded,
		Scenario51MaintenanceExpires,
		Scenario52EmergencyAccessExpires,
		Scenario53UnsupportedHardware,
		Scenario54InterfaceRenamedVM,
		Scenario55BGPRouteRejected,
		Scenario56OSPFNeighborLoss,
		Scenario57VRRPFailover,
		Scenario58LACPMemberFailure,
		Scenario59VLANQinQValidation,
		Scenario60DNSDependencyLoop,
		Scenario61CertTimeUntrusted,
		Scenario62SupportBundleRedaction,
		Scenario63FullBackupWithSecrets,
		Scenario64FactoryResetRecovery,
		Scenario65Incident30Sec,
	}
	return out
}
