package action

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ipaqsa/kube2e/internal/tools/patch"
)

// Target identifies the Kubernetes resource an action operates on.
type Target struct {
	// Object is the resource name — a key in the case-level Objects map.
	Object string `yaml:"object" json:"object"`
}

// Retry configures retry behavior for an action.
type Retry struct {
	// Attempts is the total number of executions (1 = no retry).
	Attempts int              `yaml:"attempts" json:"attempts"`
	Backoff  *metav1.Duration `yaml:"backoff" json:"backoff"`
}

// Ensure creates or updates the object on the cluster using Server-Side Apply.
// The object name is taken directly from Object; no target nesting is needed.
type Ensure struct {
	// Object is the resource name — a key in the case-level Objects map.
	Object  string           `yaml:"object" json:"object"`
	Retry   *Retry           `yaml:"retry" json:"retry"`
	Values  map[string]any   `yaml:"values" json:"values"`
	Delay   *metav1.Duration `yaml:"delay" json:"delay"`
	Timeout *metav1.Duration `yaml:"timeout" json:"timeout"`
}

// Delete removes the object from the cluster.
type Delete struct {
	actionOptions
	Retry    *Retry           `yaml:"retry" json:"retry"`
	Wait     bool             `yaml:"wait" json:"wait"`
	Interval *metav1.Duration `yaml:"interval" json:"interval"`
	Timeout  *metav1.Duration `yaml:"timeout" json:"timeout"`
}

// Patch applies RFC 6902 JSON patches to the rendered object and re-ensures it.
type Patch struct {
	actionOptions
	Retry   *Retry        `yaml:"retry" json:"retry"`
	Patches patch.Patches `yaml:"patches" json:"patches"`
}

// Wait polls the object until all JQ conditions pass or the timeout expires.
type Wait struct {
	actionOptions
	Interval   *metav1.Duration `yaml:"interval" json:"interval"`
	Conditions []string         `yaml:"conditions" json:"conditions"`
}

// Assert fetches the object once and checks that all JQ conditions evaluate to
// true. When Retry is set the check is repeated up to Retry.Attempts times with
// Retry.Backoff between each attempt.
type Assert struct {
	actionOptions
	Retry      *Retry   `yaml:"retry" json:"retry"`
	Conditions []string `yaml:"conditions" json:"conditions"`
}

type actionOptions struct {
	Target  Target           `yaml:"target" json:"target"`
	Delay   *metav1.Duration `yaml:"delay" json:"delay"`
	Timeout *metav1.Duration `yaml:"timeout" json:"timeout"`
}

// attempts returns the configured attempt count, defaulting to 1.
func (r *Retry) attempts() int {
	if r == nil || r.Attempts < 2 {
		return 1
	}

	return r.Attempts
}

// backoff returns the configured backoff duration, defaulting to zero.
func (r *Retry) backoff() time.Duration {
	if r == nil || r.Backoff == nil {
		return 0
	}

	return r.Backoff.Duration
}

// IntervalOrDefault provides the polling interval or the supplied default when
// the action does not specify one.
func (a *Wait) IntervalOrDefault(def time.Duration) time.Duration {
	if a.Interval == nil || a.Interval.Duration <= 0 {
		return def
	}

	return a.Interval.Duration
}

// IntervalOrDefault provides the delete-wait polling interval or the supplied
// default when the action does not specify one.
func (a *Delete) IntervalOrDefault(def time.Duration) time.Duration {
	if a.Interval == nil || a.Interval.Duration <= 0 {
		return def
	}

	return a.Interval.Duration
}

// TimeoutOrDefault returns the delete-wait timeout or falls back to the
// provided default when unset. Shadows actionOptions.TimeoutOrDefault so that
// the Delete-level Timeout field (not the embedded one) is read.
func (a *Delete) TimeoutOrDefault(def time.Duration) time.Duration {
	if a.Timeout == nil || a.Timeout.Duration <= 0 {
		return def
	}

	return a.Timeout.Duration
}

// TimeoutOrDefault returns the action timeout or falls back to the provided
// default when unset.
func (a *actionOptions) TimeoutOrDefault(def time.Duration) time.Duration {
	if a.Timeout == nil || a.Timeout.Duration <= 0 {
		return def
	}

	return a.Timeout.Duration
}
