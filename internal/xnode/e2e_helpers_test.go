package xnode

import (
	"encoding/pem"
	"math/big"
	"time"
)

func pemEncode(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

type bigIntAlias = big.Int

func newSerial() *big.Int { return big.NewInt(time.Now().UnixNano()) }

func pemEncodeCert(der []byte) []byte {
	return pemEncode("CERTIFICATE", der)
}

func pemEncodeKey(der []byte) []byte {
	return pemEncode("EC PRIVATE KEY", der)
}
