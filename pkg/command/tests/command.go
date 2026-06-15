// Package tests provides commands for managing kube2e test suites.
package tests

import (
	"github.com/spf13/cobra"

	cmdadd "github.com/ipaqsa/kube2e/pkg/command/tests/add"
	cmdpublish "github.com/ipaqsa/kube2e/pkg/command/tests/publish"
)

// NewTestsCommand returns the "tests" command with suite management subcommands.
func NewTestsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tests",
		Short: "Manage kube2e test suites",
		Long: `Manage kube2e test suites.

Use subcommands to scaffold, package, and publish test suites.`,
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(cmdadd.NewAddCommand())
	cmd.AddCommand(cmdpublish.NewPublishCommand())

	return cmd
}
