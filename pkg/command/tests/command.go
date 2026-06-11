// Package tests provides commands for managing kube2e test suites.
package tests

import (
	"github.com/spf13/cobra"

	cmdbuild "github.com/ipaqsa/kube2e/pkg/command/tests/build"
)

// NewTestsCommand returns the "tests" command with suite management subcommands.
func NewTestsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tests",
		Short: "Manage kube2e test suites",
		Long: `Manage kube2e test suites.

Use subcommands to package and publish test suites for remote execution.`,
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(cmdbuild.NewBuildCommand())

	return cmd
}
