package command

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func init() {
	// persistent flags for all commands
	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().String("log", "info", "Log level (debug, info, warn, error)")

	// bind persistent flags to Viper keys
	_ = viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	_ = viper.BindPFlag("log", rootCmd.PersistentFlags().Lookup("log"))

	// environment variables will be KUBE2E_KUBECONFIG, KUBE2E_LOGLEVEL
	viper.SetEnvPrefix("KUBE2E")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "kube2e",
	Short: "A CLI for e2e tests",
	Long:  `A CLI for running e2e tests against a Kubernetes cluster.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		bindFlags(cmd)
	},
}

func bindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Changed && viper.IsSet(flag.Name) {
			val := viper.Get(flag.Name)
			_ = cmd.Flags().Set(flag.Name, fmt.Sprintf("%v", val))
		}
	})
}

// Run runs the CLI
func Run() error {
	return rootCmd.Execute()
}
