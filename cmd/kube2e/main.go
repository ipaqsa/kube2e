// Package main is the entry point for the kube2e CLI.
package main

import (
	"os"

	// Register the OIDC auth provider so kubeconfigs using auth-provider: oidc
	// can build an HTTP client (client-go discovers providers via blank imports).
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/ipaqsa/kube2e/pkg/command"
)

func main() {
	if err := command.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
