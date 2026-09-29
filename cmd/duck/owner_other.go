//go:build !linux

package main

import "errors"

func ownerCmd([]string) error { return errors.New("Owner recovery is local-only on the Linux node") }
