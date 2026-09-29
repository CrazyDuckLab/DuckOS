package doctor

import (
	"errors"
	"io"
	"os"
)

// readBounded reads up to 128 KiB from a regular file. The limit
// is the same as the observe package: enough to capture any
// reasonable /proc entry, too small to absorb a runaway device.
const readLimit = 128 << 10

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, readLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > readLimit {
		return nil, errors.New("file too large")
	}
	return b, nil
}
