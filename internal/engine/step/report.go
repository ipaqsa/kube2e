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
	Duration    engine.Duration `json:"duration"`
	State       engine.State    `json:"state"`
	Reason      string          `json:"reason,omitempty"`
	Passed      int             `json:"passed"`
	Failed      int             `json:"failed,omitempty"`
	Total       int             `json:"total"`
	Stats       engine.Stats    `json:"stats"`
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
		case engine.StateFailed:
			failed++
		case engine.StateSkipped:
			// Actions are only ever passed or failed; counted as neither.
		}
	}

	return passed, failed
}

// objectStats counts the ensure, patch, and delete actions that passed.
func objectStats(reports []action.Report) engine.Stats {
	var stats engine.Stats

	for _, report := range reports {
		if report.State != engine.StatePassed {
			continue
		}

		switch report.Action {
		case action.NameEnsure:
			stats.Ensured++
		case action.NamePatch:
			stats.Patched++
		case action.NameDelete:
			stats.Deleted++
		case action.NameWait, action.NameAssert, action.NameLogs, action.NameExec:
			// Read-only actions do not mutate cluster objects.
		}
	}

	return stats
}

// failedActionReport creates a failed action report for pre-execution errors.
func failedActionReport(name action.Name, target action.Target, err error) (*action.Report, error) {
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
	report.Duration = engine.Duration(report.FinishedAt.Sub(report.StartedAt))

	if err != nil {
		report.State = engine.StateFailed
		report.Reason = err.Error()

		return report, err
	}

	report.State = engine.StatePassed

	return report, nil
}
