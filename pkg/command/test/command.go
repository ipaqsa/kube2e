// Package test provides the "test" subcommand for running kube e2e tests.
package test

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ipaqsa/kube2e/internal/tools/logs"
	"github.com/ipaqsa/kube2e/pkg/engine"
)

const (
	// testsFlag specifies which specific tests suites to run.
	testsFlag = "tests"
	// remoteFlag specifies the image with tests to run.
	remoteFlag = "remote"
	// remoteUserFlag specifies the user for image for the remote registry.
	remoteUserFlag = "remote-user"
	// remotePasswordFlag specifies the password for the remote registry.
	remotePasswordFlag = "remote-password"
)

// NewTestCommand returns the "test" cobra command that executes local or
// remote test suites.
func NewTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <dir>",
		Short: "Discover and run tests in a directory or image against a cluster",
		Long: `Walk a local directory or extracted image filesystem for test suites and
run each one against the configured cluster.

Every immediate subdirectory that contains a test.yaml file is treated as a
test suite. Use --tests to run only specific suites by name.

Kubeconfig resolution: --kubeconfig flag → $KUBECONFIG → ~/.kube/config → in-cluster.`,
		Example: `  # Run all test suites in ./examples/tests
  kube2e test ./examples/tests

  # Run only the nginx and job suites
  kube2e test ./examples/tests --tests nginx,job

  # Run with warning messages visible
  kube2e test ./examples/tests -v

  # Run remote tests from an image
  kube2e test ./tests --remote ghcr.io/tests/example:v0.0.1

  # Run a specific suite verbosely against an explicit cluster
  kube2e test ./examples/tests --tests nginx -v --kubeconfig ~/.kube/staging.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfig := viper.GetString("kubeconfig")
			verbose := viper.GetBool("verbose")

			workDir := args[0]
			remote := viper.GetString(remoteFlag)
			remoteUser := viper.GetString(remoteUserFlag)
			remotePassword := viper.GetString(remotePasswordFlag)

			// Split --tests on commas; strip whitespace and ignore empty tokens.
			var tests []string

			for t := range strings.SplitSeq(viper.GetString(testsFlag), ",") {
				if t = strings.TrimSpace(t); t != "" {
					tests = append(tests, t)
				}
			}

			restConfig, err := buildRestConfig(kubeconfig)
			if err != nil {
				return fmt.Errorf("build rest config: %w", err)
			}

			cfg := &engine.Config{
				RestConfig:     restConfig,
				WorkDir:        workDir,
				Tests:          tests,
				Remote:         remote,
				RemoteUser:     remoteUser,
				RemotePassword: remotePassword,
			}

			return engine.RunTests(cmd.Context(), cfg, logs.New(verbose))
		},
	}

	// --tests: comma-separated list of test directory names; empty means run all.
	cmd.Flags().String(testsFlag, "", "Comma-separated list of test names to run (default: all)")
	_ = viper.BindPFlag(testsFlag, cmd.Flags().Lookup("tests")) //nolint:errcheck // not need to verify it

	// --remote: image with tests to run.
	cmd.Flags().String(remoteFlag, "", "Image with tests to run")
	_ = viper.BindPFlag(remoteFlag, cmd.Flags().Lookup("remote")) //nolint:errcheck // not need to verify it

	// --remote-user: user for image for the remote registry.
	cmd.Flags().String(remoteUserFlag, "", "User for image for the remote registry")
	_ = viper.BindPFlag(remoteUserFlag, cmd.Flags().Lookup("remote-user")) //nolint:errcheck // not need to verify it

	// --remote-password: password for the remote registry.
	cmd.Flags().String(remotePasswordFlag, "", "Password for the remote registry")
	_ = viper.BindPFlag(remotePasswordFlag, cmd.Flags().Lookup("remote-password")) //nolint:errcheck // not need to verify it

	return cmd
}

// buildRestConfig constructs a *rest.Config using the standard kubeconfig
// resolution chain: explicit path → $KUBECONFIG → ~/.kube/config →
// in-cluster credentials.
func buildRestConfig(kubeconfigPath string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	return cfg, nil
}
