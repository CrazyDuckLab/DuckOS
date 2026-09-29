//go:build !linux

package main

import (
	"context"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

type linuxNetlink struct{}

func (linuxNetlink) SnapshotLinks() ([]network.ManagementLinkEvidence, []network.ManagementAddressEvidence, error) {
	return nil, nil, errors.New("netlink evidence requires Linux")
}

func (linuxNetlink) MainRoutes() ([]network.KernelRoute, error) {
	return nil, errors.New("netlink evidence requires Linux")
}

type linuxProcNet struct{}

func (linuxProcNet) Listeners() ([]network.ManagementListenerEvidence, error) {
	return nil, errors.New("proc-net evidence requires Linux")
}

type linuxLLDPProbe struct{}

func (linuxLLDPProbe) Neighbors() ([]network.ManagementNeighborEvidence, error) {
	return nil, errors.New("lldp probe requires Linux")
}

func newManagedRoutingContext() context.Context { return context.Background() }
