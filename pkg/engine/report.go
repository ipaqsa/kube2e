package engine

import (
	"time"

	core "github.com/ipaqsa/kube2e/internal/engine"
	"github.com/ipaqsa/kube2e/internal/engine/test"
)

// Report records the execution result of all discovered test directories.
type Report struct {
	DryRun     bool          `json:"dryRun"`
	Remote     *RemoteReport `json:"remote,omitempty"`
	StartedAt  time.Time     `json:"startedAt"`
	FinishedAt time.Time     `json:"finishedAt"`
	Duration   core.Duration `json:"duration"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed,omitempty"`
	Total      int           `json:"total"`
	Tests      []test.Report `json:"tests"`
}

// RemoteReport records the remote image source used for test execution.
type RemoteReport struct {
	Ref      string `json:"ref"`
	Username string `json:"username,omitempty"`
}

// countTests returns passed and failed test counts.
func countTests(reports []test.Report) (int, int) {
	var (
		passed int
		failed int
	)

	for _, report := range reports {
		switch report.State {
		case core.StatePassed, core.StateSkipped:
			passed++
		case core.StateFailed:
			failed++
		}
	}

	return passed, failed
}

// newReport creates an aggregate report initialized from the engine config.
func newReport(cfg *Config) *Report {
	report := &Report{
		DryRun:    cfg.DryRun,
		StartedAt: time.Now(),
	}
	if cfg.Remote.Ref != "" {
		report.Remote = &RemoteReport{
			Ref:      cfg.Remote.Ref,
			Username: cfg.Remote.Username,
		}
	}

	return report
}

// finishReport records the final aggregate state and returns err unchanged.
func finishReport(report *Report, err error) (*Report, error) {
	report.FinishedAt = time.Now()
	report.Duration = core.Duration(report.FinishedAt.Sub(report.StartedAt))

	return report, err
}
