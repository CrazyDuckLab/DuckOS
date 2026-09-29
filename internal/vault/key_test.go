package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationKeyProvisionLoadAndVaultReopen(t *testing.T) {
	keyPath, vaultPath := privateDirectory(t), privateDirectory(t)
	if key, err := LoadMasterKey(keyPath); err == nil {
		key.Clear()
		t.Fatal("missing key generated during load")
	}
	key, err := ProvisionMasterKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%v %#v", key, *key) != "[installation master key] [installation master key]" {
		t.Fatal("master key diagnostic disclosure")
	}
	if _, err := json.Marshal(key); err == nil {
		t.Fatal("implicit key serialization")
	}
	info, err := os.Stat(filepath.Join(keyPath, masterFilename))
	if err != nil || info.Mode().Perm() != 0600 || info.Size() != 32 {
		t.Fatal("unsafe master key file")
	}
	s, err := key.InitializeVault(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.Put("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "subscription-token", []byte("synthetic token"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	before := bytes.Clone(key.data)
	key.Clear()
	if _, err := key.OpenVault(vaultPath); err == nil {
		t.Fatal("cleared key opened vault")
	}
	if replacement, err := ProvisionMasterKey(keyPath); err == nil {
		replacement.Clear()
		t.Fatal("installation key was overwritten")
	}
	key, err = LoadMasterKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Clear()
	if !bytes.Equal(key.data, before) {
		t.Fatal("installation key changed")
	}
	s, err = key.OpenVault(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	secret, err := s.Get(ref)
	if err != nil || string(secret.Reveal()) != "synthetic token" {
		t.Fatal("loaded installation key failed to decrypt")
	}
	secret.Clear()
}

func TestInstallationKeyRejectsUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"partial", "oversized", "zero", "public", "symlink", "hardlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			path := privateDirectory(t)
			filename := filepath.Join(path, masterFilename)
			switch kind {
			case "directory":
				if err := os.Mkdir(filename, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink", "hardlink":
				target := filepath.Join(privateDirectory(t), "target")
				if err := os.WriteFile(target, bytes.Repeat([]byte{1}, 32), 0600); err != nil {
					t.Fatal(err)
				}
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(target, filename); err != nil {
					t.Fatal(err)
				}
			default:
				data := bytes.Repeat([]byte{1}, 32)
				if kind == "partial" {
					data = data[:8]
				}
				if kind == "oversized" {
					data = append(data, 1)
				}
				if kind == "zero" {
					clear(data)
				}
				if err := os.WriteFile(filename, data, 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "public" {
					if err := os.Chmod(filename, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if key, err := LoadMasterKey(path); err == nil {
				key.Clear()
				t.Fatal("unsafe key loaded")
			}
			if key, err := ProvisionMasterKey(path); err == nil {
				key.Clear()
				t.Fatal("unsafe existing key replaced")
			}
		})
	}
}

func TestInstallationKeyRequiresPrivateNonSymlinkDirectory(t *testing.T) {
	path := privateDirectory(t)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if key, err := ProvisionMasterKey(path); err == nil {
		key.Clear()
		t.Fatal("public key directory accepted")
	}
	path = privateDirectory(t)
	link := filepath.Join(privateDirectory(t), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if key, err := ProvisionMasterKey(link); err == nil {
		key.Clear()
		t.Fatal("symlink key directory accepted")
	}
}
