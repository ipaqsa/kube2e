package action

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ipaqsa/kube2e/internal/tools/patch"
)

// Action describes a single command that targets a kubernetes object.
// the command determines whether the object is ensured, deleted, waited
// upon, patched, or used to set values when the action is executed.
type Action struct {
	Command    string           `yaml:"command" json:"command"`
	Deletion   bool             `yaml:"deletion" json:"deletion"`
	Conditions []string         `yaml:"conditions" json:"conditions"`
	Delay      *metav1.Duration `yaml:"delay" json:"delay"`
	Interval   *metav1.Duration `yaml:"interval" json:"interval"`
	Timeout    *metav1.Duration `yaml:"timeout" json:"timeout"`
	Patches    patch.Patches    `yaml:"patches" json:"patches"`

	// Values are merged with the object name to render the template.
	// Only meaningful for the Ensure command; ignored by all other commands.
	Values map[string]any `yaml:"values" json:"values"`

	// Fields for Value command
	ValueKey  string `yaml:"valueKey" json:"valueKey"`
	ValuePath string `yaml:"valuePath" json:"valuePath"`
}

// String returns a lowercase representation of the action command and template.
func (a *Action) String(object string) string {
	return fmt.Sprintf("%s.%s", strings.ToLower(a.Command), object)
}

// IntervalOrDefault provides the polling interval or the supplied default when
// the action does not specify one.
func (a *Action) IntervalOrDefault(def time.Duration) time.Duration {
	if a.Interval == nil || a.Interval.Duration <= 0 {
		return def
	}

	return a.Interval.Duration
}

// TimeoutOrDefault returns the action timeout or falls back to the provided
// default when unset.
func (a *Action) TimeoutOrDefault(def time.Duration) time.Duration {
	if a.Timeout == nil || a.Timeout.Duration <= 0 {
		return def
	}

	return a.Timeout.Duration
}
