//go:build mage
// +build mage

package main

import (
	"github.com/magefile/mage/sh"
)

const (
	K3S_IMAGE    = "rancher/k3s:v1.34.3-k3s1"
	CLUSTER_NAME = "nd-e2e"
	AGENTS       = "4"
)

// Runs the End-to-End Test in a local Kind Cluster
func TestE2E() error {
	env := map[string]string{
		"KUBECONFIG": "/tmp/nd-e2e-kubeconfig.yaml",
		CLUSTER_NAME: CLUSTER_NAME,
	}
	k3dOptions := []string{"cluster", "create", CLUSTER_NAME, "--no-lb", "--image", K3S_IMAGE, "--agents", AGENTS}
	k3dOptions = append(k3dOptions, "--k3s-arg", "--disable=traefik,servicelb,metrics-serve@server:0")
	k3dOptions = append(k3dOptions, "--wait")
	err := sh.RunWithV(env, "k3d", k3dOptions...)
	if err != nil {
		return err
	}
	// make sure we delete the kind cluster
	defer sh.RunWithV(env, "k3d", "cluster", "delete", CLUSTER_NAME)

	err = sh.RunWithV(env, "go", "test", "./test/e2e/", "-v", "-ginkgo.v")
	if err != nil {
		return err
	}
	return nil
}
