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
	// tagsFlag filters test suites and cases by tag (comma-separated list).
	tagsFlag = "tags"
	// parallelFlag controls how many test suites run concurrently.
	parallelFlag = "parallel"
	// remoteFlag specifies the image with tests to run.
	remoteFlag = "remote"
	// remoteUserFlag specifies the user for image for the remote registry.
	remoteUserFlag = "remote-user"
	// remotePasswordFlag specifies the password for the remote registry.
	remotePasswordFlag = "remote-password"
	// dryRunFlag specifies whether to run in dry run mode (no resources are applied).
	dryRunFlag = "dry-run"
)

// NewRunCommand returns the "run" cobra command that executes local or remote test suites.
func NewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <dir>",
		Short: "Discover and run tests in a directory or image against a cluster",
		Long: `Walk a local directory or extracted image filesystem for test suites and
run each one against the configured cluster.

Every immediate subdirectory that contains a test.yaml file is treated as a
test suite. Use --tags to run only suites and cases that carry matching tags.

Kubeconfig resolution: --kubeconfig flag -> $KUBECONFIG -> ~/.kube/config -> in-cluster.`,
		Example: `  # Run all test suites in ./examples/tests
  kube2e run ./examples/tests

  # Run only tests and cases tagged "smoke" or "aws"
  kube2e run ./examples/tests --tags smoke,aws

  # Run 4 test suites in parallel
  kube2e run ./examples/tests -n 4

  # Run in dry run mode
  kube2e run ./examples/tests --dry-run

  # Run remote tests from an image
  kube2e run ./tests --remote ghcr.io/tests/example:v0.0.1

  # Run tagged tests against an explicit cluster
  kube2e run ./examples/tests --tags smoke --kubeconfig ~/.kube/staging.yaml`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	// --tags: comma-separated list of tags; empty means run all.
	cmd.Flags().String(tagsFlag, "", "Comma-separated list of tags to run (default: all)")
	_ = viper.BindPFlag(tagsFlag, cmd.Flags().Lookup(tagsFlag)) //nolint:errcheck // not need to verify it

	// -n / --parallel: number of concurrent test suites (default: 1 = sequential).
	cmd.Flags().IntP(parallelFlag, "n", 1, "Number of test suites to run concurrently")
	_ = viper.BindPFlag(parallelFlag, cmd.Flags().Lookup(parallelFlag)) //nolint:errcheck // not need to verify it

	// --remote: image with tests to run.
	cmd.Flags().String(remoteFlag, "", "Image with tests to run")
	_ = viper.BindPFlag(remoteFlag, cmd.Flags().Lookup(remoteFlag)) //nolint:errcheck // not need to verify it

	// --remote-user: user for image for the remote registry.
	cmd.Flags().String(remoteUserFlag, "", "User for image for the remote registry")
	_ = viper.BindPFlag(remoteUserFlag, cmd.Flags().Lookup(remoteUserFlag)) //nolint:errcheck // not need to verify it

	// --remote-password: password for image for the remote registry.
	cmd.Flags().String(remotePasswordFlag, "", "Password for the remote registry")
	_ = viper.BindPFlag(remotePasswordFlag, cmd.Flags().Lookup(remotePasswordFlag)) //nolint:errcheck // not need to verify it

	// --dry-run: run in dry run mode (no resources are applied).
	cmd.Flags().Bool(dryRunFlag, false, "Run in dry run mode")
	_ = viper.BindPFlag(dryRunFlag, cmd.Flags().Lookup(dryRunFlag)) //nolint:errcheck // not need to verify it

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

	tags := splitTags(viper.GetString(tagsFlag))
	parallel := viper.GetInt(parallelFlag)

	dryRun := viper.GetBool(dryRunFlag)

	var restConfig *rest.Config
	if !dryRun {
		var err error
		restConfig, err = buildRestConfig(kubeconfig)
		if err != nil {
			return fmt.Errorf("build rest config: %w", err)
		}
	}

	logger := logs.New(verbose)
	cfg := &engine.Config{
		RestConfig: restConfig,
		WorkDir:    workDir,
		Tags:       tags,
		Parallel:   parallel,
		DryRun:     dryRun,
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

// splitTags splits a comma-separated tag list, trimming spaces.
func splitTags(value string) []string {
	var tags []string

	for t := range strings.SplitSeq(value, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}

	return tags
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
