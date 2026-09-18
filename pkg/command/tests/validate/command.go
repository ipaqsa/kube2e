// Package validate provides the "tests validate" subcommand.
package validate

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	validation "github.com/ipaqsa/kube2e/internal/validate"
)

const (
	// okMark prefixes a case file that matched the schema.
	okMark = "ok  "
	// failMark prefixes a case file that did not.
	failMark = "FAIL"
)

// NewValidateCommand returns the "tests validate" command.
func NewValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <dir>",
		Short: "Validate case files against the case schema",
		Long: `Validate kube2e case files against the built-in case JSON Schema.

<dir> is either a test suite directory (one that contains cases/) or a parent
directory whose immediate children are test suites. Every cases/*.yaml file is
checked and the cluster is never contacted.

The schema is stricter than the case parser: it also rejects values the parser
accepts, such as a malformed timeout or an unknown logs match policy. The
command exits non-zero when any case file is invalid.`,
		Example: `  # Validate every suite under ./examples
  kube2e tests validate ./examples

  # Validate a single suite
  kube2e tests validate ./examples/nginx`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	return cmd
}

// run validates the case files of the directory given as the first argument.
func run(cmd *cobra.Command, args []string) error {
	validator, err := validation.New()
	if err != nil {
		return fmt.Errorf("create validator: %w", err)
	}

	report, err := validator.Dir(args[0])
	if err != nil {
		return fmt.Errorf("validate tests: %w", err)
	}

	if err = printReport(cmd.OutOrStdout(), report); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	total, invalid := report.Totals()
	if invalid > 0 {
		return fmt.Errorf("%d of %d case files are invalid", invalid, total)
	}

	return nil
}

// printReport writes a human-readable summary of report to out.
func printReport(out io.Writer, report *validation.Report) error {
	for _, suite := range report.Suites {
		if _, err := fmt.Fprintln(out, suite.Path); err != nil {
			return err
		}

		for _, caseReport := range suite.Cases {
			if err := printCase(out, suite.Path, caseReport); err != nil {
				return err
			}
		}
	}

	total, invalid := report.Totals()

	if _, err := fmt.Fprintf(out, "\n%d case files checked, %d invalid\n", total, invalid); err != nil {
		return err
	}

	return nil
}

// printCase writes the result of a single case file, indented under its suite.
func printCase(out io.Writer, suiteDir string, report validation.CaseReport) error {
	mark := okMark
	if !report.Valid() {
		mark = failMark
	}

	if _, err := fmt.Fprintf(out, "  %s %s\n", mark, relative(suiteDir, report.Path)); err != nil {
		return err
	}

	for _, problem := range report.Problems {
		if _, err := fmt.Fprintf(out, "       %s: %s\n", problem.Pointer, problem.Message); err != nil {
			return err
		}
	}

	return nil
}

// relative returns path relative to base, falling back to path itself.
func relative(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}

	return rel
}
