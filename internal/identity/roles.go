package identity

type Role string

const (
	Owner        Role = "owner"
	NetworkAdmin Role = "network-admin"
	Helpdesk     Role = "helpdesk"
	Reception    Role = "reception"
	ReadOnly     Role = "read-only"
)

type Permission string

const (
	Observe          Permission = "observe"
	ConfigureNetwork Permission = "network.write"
	Diagnose         Permission = "diagnose"
	ManageGuests     Permission = "guests.write"
	ManageUsers      Permission = "users.write"
	ManageSecurity   Permission = "security.write"
	ManageUpdates    Permission = "updates.write"
	ManageBackups    Permission = "backups.write"
	ManageSecrets    Permission = "secrets.write"
	ExportClients    Permission = "clients.export"
)

func validPermission(p Permission) bool {
	switch p {
	case Observe, ConfigureNetwork, Diagnose, ManageGuests, ManageUsers, ManageSecurity, ManageUpdates, ManageBackups, ManageSecrets, ExportClients:
		return true
	default:
		return false
	}
}

func Allowed(role Role, p Permission) bool {
	switch p {
	case Observe:
		return role == Owner || role == NetworkAdmin || role == Helpdesk || role == ReadOnly
	case ConfigureNetwork, ExportClients:
		return role == Owner || role == NetworkAdmin
	case Diagnose:
		return role == Owner || role == NetworkAdmin || role == Helpdesk
	case ManageGuests:
		return role == Owner || role == NetworkAdmin || role == Reception
	case ManageUsers, ManageSecurity, ManageUpdates, ManageBackups, ManageSecrets:
		return role == Owner
	default:
		return false
	}
}
func validRole(role Role) bool {
	return role == Owner || role == NetworkAdmin || role == Helpdesk || role == Reception || role == ReadOnly
}
