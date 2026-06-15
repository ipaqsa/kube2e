// Package main is the entry point for the kube2e CLI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	// Register the OIDC auth provider so kubeconfigs using auth-provider: oidc
	// can build an HTTP client (client-go discovers providers via blank imports).
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/ipaqsa/kube2e/pkg/command"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

// run wires interrupt handling and executes the root command. It exists so the
// deferred signal-stop runs before main calls os.Exit on failure.
func run() error {
	// Cancel the root context on SIGINT/SIGTERM so execution unwinds gracefully;
	// a second signal restores default behavior and force-quits the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return command.NewRootCommand().ExecuteContext(ctx)
}
