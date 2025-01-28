//go:build mage
// +build mage

package main

import (
	"github.com/magefile/mage/sh"
)

// Runs the End-to-End Test in a local Kind Cluster
func TestE2E() error {
	err := sh.RunV("kind", "create", "cluster", "--config", "kind-config-e2e.yaml")
	if err != nil {
		return err
	}

	// make sure we delete the kind cluster
	defer sh.RunV("kind", "delete", "cluster")
	return sh.RunV("make", "test-e2e")
}
