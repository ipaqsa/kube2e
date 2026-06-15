package action

import (
	"time"

	"github.com/ipaqsa/kube2e/internal/engine"
)

// Report records the execution result of a single action.
type Report struct {
	Action     Name            `json:"action"`
	Target     Target          `json:"target"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt time.Time       `json:"finishedAt"`
	Duration   engine.Duration `json:"duration"`
	State      engine.State    `json:"state"`
	Reason     string          `json:"reason,omitempty"`
}

// newReport creates a report initialized with the action identity and start time.
func newReport(name Name, target Target) *Report {
	return &Report{
		Action:    name,
		Target:    target,
		StartedAt: time.Now(),
	}
}

// finishReport records the final action state and returns err unchanged.
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
