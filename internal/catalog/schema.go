package catalog

const applicationID = 0x4455434b
const schemaVersion = 1
const maxDatabaseBytes = 32 << 20

var schema = map[string]string{
	"clients": `CREATE TABLE clients (
id TEXT PRIMARY KEY CHECK(length(id)=32),
revision INTEGER NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
document TEXT NOT NULL CHECK(length(document)<=8192)
) WITHOUT ROWID`,
	"client_history": `CREATE TABLE client_history (
client_id TEXT NOT NULL REFERENCES clients(id),
revision INTEGER NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
document TEXT NOT NULL CHECK(length(document)<=8192),
PRIMARY KEY(client_id,revision)
) WITHOUT ROWID`,
	"catalog_audit": `CREATE TABLE catalog_audit (
sequence INTEGER PRIMARY KEY,
actor TEXT NOT NULL CHECK(length(actor)=32),
client_id TEXT NOT NULL CHECK(length(client_id)=32),
action TEXT NOT NULL CHECK(action IN ('client.create','client.update')),
revision INTEGER NOT NULL,
at TEXT NOT NULL
)`,
}
