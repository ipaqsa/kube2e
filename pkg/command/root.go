// Package command wires together the top-level cobra command hierarchy.
package command

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/ipaqsa/kube2e/internal/version"
	cmdrun "github.com/ipaqsa/kube2e/pkg/command/run"
	cmdtests "github.com/ipaqsa/kube2e/pkg/command/tests"
	cmdversion "github.com/ipaqsa/kube2e/pkg/command/version"
)

// NewRootCommand returns the root cobra command with all subcommands registered.
// Persistent flags are bound to Viper so they can also be set via environment
// variables: KUBE2E_KUBECONFIG, KUBE2E_VERBOSE.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "kube2e",
		Short:   "YAML-driven end-to-end testing for Kubernetes",
		Version: version.Get().String(),
		Long: `kube2e discovers test suites in a directory and runs them against a live
Kubernetes cluster using declarative YAML scenarios.

Each test suite contains a test.yaml descriptor, Go templates, and case files
that define ordered steps and actions (Ensure, Delete, Wait, Patch, Value).

CRDs must be provisioned in the cluster before running tests.`,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			bindFlags(cmd)
		},
	}

	// --kubeconfig: explicit path; falls back to $KUBECONFIG then ~/.kube/config.
	cmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig file ($KUBECONFIG or ~/.kube/config used when not set)")
	// --verbose / -v: include Warn-level log records in output.
	cmd.PersistentFlags().BoolP("verbose", "v", false, "Show warning-level log messages in addition to info and error")

	_ = viper.BindPFlag("kubeconfig", cmd.PersistentFlags().Lookup("kubeconfig")) //nolint:errcheck // not need to verify it
	_ = viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))       //nolint:errcheck // not need to verify it

	// Environment variable prefix: KUBE2E_KUBECONFIG, KUBE2E_VERBOSE.
	viper.SetEnvPrefix("KUBE2E")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	cmd.AddCommand(cmdrun.NewRunCommand())
	cmd.AddCommand(cmdtests.NewTestsCommand())
	cmd.AddCommand(cmdversion.NewVersionCommand())

	return cmd
}

// bindFlags syncs Viper values back into cobra flags so that environment
// variables override flag defaults without requiring explicit --flag usage.
func bindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Changed && viper.IsSet(flag.Name) {
			val := viper.Get(flag.Name)
			_ = cmd.Flags().Set(flag.Name, fmt.Sprintf("%v", val)) //nolint:errcheck // not need to verify it
		}
	})
}
