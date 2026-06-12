package step

import (
	"time"

	"github.com/ipaqsa/kube2e/internal/engine"
	"github.com/ipaqsa/kube2e/internal/engine/action"
)

// Report records the execution result of a single step and its actions.
type Report struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Optional    bool            `json:"optional"`
	StartedAt   time.Time       `json:"startedAt"`
	FinishedAt  time.Time       `json:"finishedAt"`
	State       engine.State    `json:"state"`
	Reason      string          `json:"reason,omitempty"`
	Passed      int             `json:"passed"`
	Failed      int             `json:"failed,omitempty"`
	Total       int             `json:"total"`
	Actions     []action.Report `json:"actions"`
}

// appendActionReport appends report when present and returns err unchanged.
func appendActionReport(reports *[]action.Report, report *action.Report, err error) error {
	if report != nil {
		*reports = append(*reports, *report)
	}

	return err
}

// countActions returns passed and failed action counts.
func countActions(reports []action.Report) (int, int) {
	var (
		passed int
		failed int
	)

	for _, report := range reports {
		switch report.State {
		case engine.StatePassed:
			passed++
		case engine.StateSkipped:
		case engine.StateFailed:
			failed++
		}
	}

	return passed, failed
}

// failedActionReport creates a failed action report for pre-execution errors.
func failedActionReport(name string, target action.Target, err error) (*action.Report, error) {
	now := time.Now()

	return &action.Report{
		Action:     name,
		Target:     target,
		StartedAt:  now,
		FinishedAt: now,
		State:      engine.StateFailed,
		Reason:     err.Error(),
	}, err
}

// finishReport records the final step state and returns err unchanged.
func finishReport(report *Report, err error) (*Report, error) {
	report.FinishedAt = time.Now()
	if err != nil {
		report.State = engine.StateFailed
		report.Reason = err.Error()

		return report, err
	}

	report.State = engine.StatePassed

	return report, nil
}
