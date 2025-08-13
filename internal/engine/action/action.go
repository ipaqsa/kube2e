package action

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kube2e/internal/tools/patch"
)

// action describes a single command that targets a kubernetes object.
// the command determines whether the object is ensured, deleted, waited
// upon or patched when the action is executed.
type Action struct {
	Command   string           `yaml:"command" json:"command"`
	Object    Object           `yaml:"object" json:"object"`
	Condition string           `yaml:"condition" json:"condition"`
	Deletion  bool             `yaml:"deletion" json:"deletion"`
	Interval  *metav1.Duration `yaml:"interval" json:"interval"`
	Timeout   *metav1.Duration `yaml:"timeout" json:"timeout"`
	Patches   patch.Patches    `yaml:"patches" json:"patches"`
}

// object references a template and optional values used to render a resource
// for the command.
type Object struct {
	Template string                 `yaml:"template" json:"template"`
	Values   map[string]interface{} `yaml:"values" json:"values"`
}

// string returns a lowercase representation of the action command and template.
func (a *Action) String() string {
	return fmt.Sprintf("%s.%s", strings.ToLower(a.Command), a.Object.Template)
}

// intervalOrDefault provides the polling interval or the supplied default when
// the action does not specify one.
func (a *Action) IntervalOrDefault(def time.Duration) time.Duration {
	if a.Interval == nil || a.Interval.Duration <= 0 {
		return def
	}

	return a.Interval.Duration
}

// timeoutOrDefault returns the action timeout or falls back to the provided
// default when unset.
func (a *Action) TimeoutOrDefault(def time.Duration) time.Duration {
	if a.Timeout == nil || a.Timeout.Duration <= 0 {
		return def
	}

	return a.Timeout.Duration
}
