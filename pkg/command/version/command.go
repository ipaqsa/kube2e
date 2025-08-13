// Package version provides the "version" subcommand.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ipaqsa/kube2e/internal/version"
)

// NewVersionCommand returns a cobra command that prints build version information.
func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Long:  "Print kube2e version, platform, architecture, and Go toolchain version.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), version.Get().String()); err != nil {
				return fmt.Errorf("write version: %w", err)
			}

			return nil
		},
	}
}
