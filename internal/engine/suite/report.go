package suite

import (
	"time"

	"github.com/ipaqsa/kube2e/internal/engine"
	"github.com/ipaqsa/kube2e/internal/engine/step"
)

// Report records the execution result of a case file and its steps.
type Report struct {
	Version     string            `json:"version"`
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Objects     map[string]string `json:"objects,omitempty"`
	State       engine.State      `json:"state"`
	Reason      string            `json:"reason,omitempty"`
	StartedAt   time.Time         `json:"startedAt"`
	FinishedAt  time.Time         `json:"finishedAt"`
	Hooks       *HooksReport      `json:"hooks,omitempty"`
	Passed      int               `json:"passed"`
	Failed      int               `json:"failed,omitempty"`
	Total       int               `json:"total"`
	Steps       []step.Report     `json:"steps"`
}

// HooksReport records the execution results of case-level hooks.
type HooksReport struct {
	BeforeEach []step.Report `json:"beforeEach,omitempty"`
	AfterEach  []step.Report `json:"afterEach,omitempty"`
}

// appendHooksReport appends hook execution reports to the case report.
func appendHooksReport(report *Report, hooks *HooksReport) {
	if hooks == nil {
		return
	}

	if len(hooks.BeforeEach) == 0 && len(hooks.AfterEach) == 0 {
		return
	}

	if report.Hooks == nil {
		report.Hooks = new(HooksReport)
	}

	report.Hooks.BeforeEach = append(report.Hooks.BeforeEach, hooks.BeforeEach...)
	report.Hooks.AfterEach = append(report.Hooks.AfterEach, hooks.AfterEach...)
}

// countSteps returns passed and failed main step counts.
func countSteps(reports []step.Report) (int, int) {
	var (
		passed int
		failed int
	)

	for _, report := range reports {
		switch report.State {
		case engine.StatePassed, engine.StateSkipped:
			passed++
		case engine.StateFailed:
			failed++
		}
	}

	return passed, failed
}

// newReport creates a case report initialized from case metadata.
func newReport(testCase *Case) *Report {
	report := &Report{StartedAt: time.Now()}
	if testCase == nil {
		return report
	}

	report.Version = testCase.Version
	report.Path = testCase.Path
	report.Name = testCase.Name
	report.Description = testCase.Description
	report.Namespace = testCase.Namespace
	report.Tags = testCase.Tags
	report.Objects = testCase.Objects
	report.Total = len(testCase.Steps)

	return report
}

// finishReport records the final case state and returns err unchanged.
func finishReport(report *Report, err error) (*Report, error) {
	report.FinishedAt = time.Now()
	if err != nil {
		report.State = engine.StateFailed
		report.Reason = err.Error()

		return report, err
	}

	if report.State == "" {
		report.State = engine.StatePassed
	}

	return report, nil
}
