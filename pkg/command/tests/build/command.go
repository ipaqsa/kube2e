// Package build provides the "tests build" subcommand.
package build

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ipaqsa/kube2e/internal/image"
	"github.com/ipaqsa/kube2e/internal/tools/logs"
)

const (
	// remoteEnv names the environment variable for the image to push.
	remoteEnv = "KUBE2E_TESTS_BUILD_REMOTE"
	// remoteUserEnv names the environment variable for the registry username.
	remoteUserEnv = "KUBE2E_TESTS_BUILD_REMOTE_USER"
	// remotePasswordEnv names the environment variable for the registry password.
	remotePasswordEnv = "KUBE2E_TESTS_BUILD_REMOTE_PASSWORD" //nolint:gosec // this is an environment variable name, not a credential value
)

var (
	// remote stores the image reference to push.
	remote string
	// remoteUser stores the registry username.
	remoteUser string
	// remotePassword stores the registry password.
	remotePassword string
)

// NewBuildCommand returns the "tests build" command.
func NewBuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <dir>",
		Short: "Package test suites into an image and push it",
		Long: `Package kube2e test suites into a container image and push it to a registry.

Only immediate child directories containing test.yaml are included. Registry
credentials can be supplied with flags or environment variables:
KUBE2E_TESTS_BUILD_REMOTE, KUBE2E_TESTS_BUILD_REMOTE_USER, and
KUBE2E_TESTS_BUILD_REMOTE_PASSWORD. When the username is omitted, kube2e uses
the default Docker credential keychain.`,
		Example: `  # Build and push all test suites under ./examples/tests
  kube2e tests build ./examples/tests --remote ghcr.io/example/kube2e-tests:v0.1.0

  # Build and push using explicit registry credentials
  kube2e tests build ./examples/tests --remote ghcr.io/example/kube2e-tests:v0.1.0 --remote-user "$USER" --remote-password "$TOKEN"`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	cmd.Flags().StringVar(&remote, "remote", "", "Image reference to push (env: KUBE2E_TESTS_BUILD_REMOTE)")
	cmd.Flags().StringVar(&remoteUser, "remote-user", "", "Registry username (env: KUBE2E_TESTS_BUILD_REMOTE_USER)")
	cmd.Flags().StringVar(&remotePassword, "remote-password", "", "Registry password (env: KUBE2E_TESTS_BUILD_REMOTE_PASSWORD)")

	return cmd
}

// run packages test suites and pushes the resulting image.
func run(cmd *cobra.Command, args []string) error {
	r := image.Remote{
		Ref:      valueOrEnv(remote, remoteEnv),
		Username: valueOrEnv(remoteUser, remoteUserEnv),
		Password: valueOrEnv(remotePassword, remotePasswordEnv),
	}
	if r.Ref == "" {
		return fmt.Errorf("remote image is required: set --remote or %s", remoteEnv)
	}

	if err := image.Build(cmd.Context(), r, args[0], logs.New(viper.GetBool("verbose"))); err != nil {
		return fmt.Errorf("build tests image: %w", err)
	}

	return nil
}

// valueOrEnv returns value when set, otherwise the value of env.
func valueOrEnv(value, env string) string {
	if value != "" {
		return value
	}

	return os.Getenv(env)
}
