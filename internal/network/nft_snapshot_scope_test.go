package network

import "testing"

func TestDecodeNFTSnapshotForTableScopesOwnership(t *testing.T) {
	fw := `{"nftables":[{"metainfo":{"version":"1.0.9"}},` +
		`{"table":{"family":"inet","name":"duckos_fw"}},` +
		`{"chain":{"family":"inet","table":"duckos_fw","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"accept"}},` +
		`{"rule":{"family":"inet","table":"duckos_fw","chain":"forward","handle":3,"expr":[{"verdict":{"verdict":"accept"}}]}}]}`
	if _, err := DecodeNFTSnapshotForTable([]byte(fw), "duckos_fw"); err != nil {
		t.Fatalf("owned duckos_fw snapshot rejected: %v", err)
	}
	if _, err := DecodeNFTSnapshotForTable([]byte(fw), "duckos"); err == nil {
		t.Fatal("duckos decoder must reject a duckos_fw snapshot")
	}
	if _, err := DecodeNFTSnapshot([]byte(fw)); err == nil {
		t.Fatal("DecodeNFTSnapshot must stay scoped to the network table")
	}
	net := `{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`
	if _, err := DecodeNFTSnapshot([]byte(net)); err != nil {
		t.Fatalf("network snapshot rejected: %v", err)
	}
}
