// Package main is the entry point for the kube2e CLI.
package main

import (
	"os"

	"github.com/ipaqsa/kube2e/pkg/command"
)

func main() {
	if err := command.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
