package catalog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func privateRoot(path string) (*os.Root, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, "", ErrUnavailable
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return nil, "", ErrUnavailable
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, "", ErrUnavailable
	}
	return root, abs, nil
}

func privateFile(root *os.Root, name string, optional bool) (os.FileInfo, error) {
	info, err := root.Lstat(name)
	if optional && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 2*maxDatabaseBytes {
		return nil, ErrUnavailable
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() || st.Nlink != 1 {
		return nil, ErrUnavailable
	}
	return info, nil
}

func existingHeader(root *os.Root) (os.FileInfo, error) {
	info, err := privateFile(root, "catalog.db", false)
	if err != nil || info.Size() < 100 || info.Size() > maxDatabaseBytes {
		return nil, ErrUnavailable
	}
	f, err := root.Open("catalog.db")
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, ErrUnavailable
	}
	header := make([]byte, 100)
	if _, err := io.ReadFull(f, header); err != nil || !bytes.Equal(header[:16], []byte("SQLite format 3\x00")) || binary.BigEndian.Uint32(header[68:72]) != applicationID || binary.BigEndian.Uint32(header[60:64]) != schemaVersion || binary.BigEndian.Uint16(header[16:18]) != 4096 {
		return nil, ErrUnavailable
	}
	return info, nil
}
