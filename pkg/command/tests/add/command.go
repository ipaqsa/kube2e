// Package add provides the "tests add" subcommand.
package add

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ipaqsa/kube2e/internal/scaffold"
)

// dir is the parent directory the new suite is created in.
var dir string

// NewAddCommand returns the "tests add" command.
func NewAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Scaffold a new test suite directory",
		Long: `Scaffold a new test suite directory named <name>.

Creates <name>/cases/ and <name>/templates/ with a starter ConfigMap template and
a starter case. The case's optional fields are written as comments, so you can
uncomment the ones you need.`,
		Example: `  # Create ./nginx with a starter case and template
  kube2e tests add nginx

  # Create the suite under ./examples
  kube2e tests add nginx --dir ./examples`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	cmd.Flags().StringVarP(&dir, "dir", "C", ".", "Parent directory to create the suite in")

	return cmd
}

// run scaffolds the suite directory from the command arguments.
func run(cmd *cobra.Command, args []string) error {
	path, err := scaffold.Create(scaffold.Config{Dir: dir, Name: args[0]})
	if err != nil {
		return fmt.Errorf("add test suite: %w", err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created test suite %q\n", path); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}
