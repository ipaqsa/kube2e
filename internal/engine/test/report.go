package test

import (
	"time"

	"github.com/ipaqsa/kube2e/internal/engine"
	"github.com/ipaqsa/kube2e/internal/engine/suite"
)

// Report records the execution result of a test directory and its cases.
type Report struct {
	Path       string          `json:"path"`
	Name       string          `json:"name"`
	Passed     int             `json:"passed"`
	Failed     int             `json:"failed,omitempty"`
	Total      int             `json:"total"`
	State      engine.State    `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt time.Time       `json:"finishedAt"`
	Duration   engine.Duration `json:"duration"`
	Stats      engine.Stats    `json:"stats"`
	Cases      []suite.Report  `json:"cases"`
}

// newReport creates a test report initialized from test metadata.
func newReport(test *Test) *Report {
	return &Report{
		Path:      test.Path,
		Name:      test.Name,
		StartedAt: time.Now(),
	}
}

// finishReport records the final test state and returns err unchanged.
func finishReport(report *Report, err error) (*Report, error) {
	report.FinishedAt = time.Now()
	report.Duration = engine.Duration(report.FinishedAt.Sub(report.StartedAt))
	report.Total = len(report.Cases)
	report.Stats = sumStats(report.Cases)

	if err != nil {
		report.State = engine.StateFailed
		if report.Reason == "" {
			report.Reason = err.Error()
		}

		return report, err
	}

	report.State = engine.StatePassed

	return report, nil
}
