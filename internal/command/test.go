package command

import (
	managertest "kube2e/internal/manager/test"
	"kube2e/internal/tools/logs"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	rootCmd.AddCommand(testCmd)

	testCmd.Flags().String("test", "", "Specific test to run")
	testCmd.Flags().String("case", "", "Specific case within the test")

	_ = viper.BindPFlag("test", testCmd.Flags().Lookup("test"))
	_ = viper.BindPFlag("case", testCmd.Flags().Lookup("case"))
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run tests",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kubeconfig := viper.GetString("kubeconfig")
		logLevel := viper.GetString("log")
		testName := viper.GetString("test")
		caseName := viper.GetString("case")
		testDir := args[0]

		cfg := &managertest.Config{
			Kubeconfig:   kubeconfig,
			SpecificTest: testName,
			SpecificCase: caseName,
			TestDir:      testDir,
		}

		logger := logs.New(logLevel)

		// pretend test passed
		return managertest.NewManager(logger).Run(cmd.Context(), cfg)
	},
}
