// Package run provides the "run" subcommand for running kube e2e tests.
package run

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ipaqsa/kube2e/internal/image"
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

// NewRunCommand returns the "run" cobra command that executes local or remote test suites.
func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <dir>",
		Short: "Discover and run tests in a directory or image against a cluster",
		Long: `Walk a local directory or extracted image filesystem for test suites and
run each one against the configured cluster.

Every immediate subdirectory that contains a test.yaml file is treated as a
test suite. Use --tests to run only specific suites by name.

Kubeconfig resolution: --kubeconfig flag -> $KUBECONFIG -> ~/.kube/config -> in-cluster.`,
		Example: `  # Run all test suites in ./examples/tests
  kube2e run ./examples/tests

  # Run only the nginx and job suites
  kube2e run ./examples/tests --tests nginx,job

  # Run with warning messages visible
  kube2e run ./examples/tests -v

  # Run remote tests from an image
  kube2e run ./tests --remote ghcr.io/tests/example:v0.0.1

  # Run a specific suite verbosely against an explicit cluster
  kube2e run ./examples/tests --tests nginx -v --kubeconfig ~/.kube/staging.yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
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

	// --remote-password: password for image for the remote registry.
	cmd.Flags().String(remotePasswordFlag, "", "Password for the remote registry")
	_ = viper.BindPFlag(remotePasswordFlag, cmd.Flags().Lookup("remote-password")) //nolint:errcheck // not need to verify it

	return cmd
}

// run executes local or remote test suites using command flags.
func run(cmd *cobra.Command, args []string) error {
	kubeconfig := viper.GetString("kubeconfig")
	verbose := viper.GetBool("verbose")

	workDir := args[0]
	remote := viper.GetString(remoteFlag)
	remoteUser := viper.GetString(remoteUserFlag)
	remotePassword := viper.GetString(remotePasswordFlag)

	tests := splitTests(viper.GetString(testsFlag))

	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	logger := logs.New(verbose)
	cfg := &engine.Config{
		RestConfig: restConfig,
		WorkDir:    workDir,
		Tests:      tests,
	}

	if remote != "" {
		r := image.Remote{
			Ref:      remote,
			Username: remoteUser,
			Password: remotePassword,
		}

		return runRemote(cmd, cfg, r, logger)
	}

	return engine.RunTests(cmd.Context(), cfg, logger)
}

// runRemote extracts tests from r and runs the engine against the extracted directory.
func runRemote(cmd *cobra.Command, cfg *engine.Config, r image.Remote, logger *slog.Logger) error {
	logger.Info("pull tests image", "image", r.Ref)

	tester := func(dir string) error {
		next := *cfg
		next.WorkDir = filepath.Join(dir, cfg.WorkDir)

		return engine.RunTests(cmd.Context(), &next, logger)
	}

	if err := image.Traverse(cmd.Context(), r, tester); err != nil {
		return fmt.Errorf("traverse image: %w", err)
	}

	return nil
}

// splitTests splits a comma-separated test allowlist.
func splitTests(value string) []string {
	var tests []string

	for t := range strings.SplitSeq(value, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tests = append(tests, t)
		}
	}

	return tests
}

// buildRestConfig constructs a *rest.Config using the standard kubeconfig resolution chain.
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
