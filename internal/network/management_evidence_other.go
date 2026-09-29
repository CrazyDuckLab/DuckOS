//go:build !linux

package network

import "context"

func collectManagementEvidence(_ ManagementEvidenceCollector) (ManagementEvidence, error) {
	return ManagementEvidence{}, errUnsupportedManagementEvidence
}

var errUnsupportedManagementEvidence = &unsupportedError{}

type unsupportedError struct{}

func (*unsupportedError) Error() string {
	return "management evidence collection requires a Linux root-enrolled Agent"
}

var _ = context.Background
