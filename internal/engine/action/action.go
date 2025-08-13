package action

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kube2e/internal/tools/patch"
)

type Action struct {
	Command   string           `yaml:"command" json:"command"`
	Object    Object           `yaml:"object" json:"object"`
	Condition string           `yaml:"condition" json:"condition"`
	Deletion  bool             `yaml:"deletion" json:"deletion"`
	Interval  *metav1.Duration `yaml:"interval" json:"interval"`
	Timeout   *metav1.Duration `yaml:"timeout" json:"timeout"`
	Patches   patch.Patches    `yaml:"patches" json:"patches"`
}

type Object struct {
	Template string                 `yaml:"template" json:"template"`
	Values   map[string]interface{} `yaml:"values" json:"values"`
}

func (a *Action) String() string {
	return fmt.Sprintf("%s.%s", strings.ToLower(a.Command), a.Object.Template)
}

func (a *Action) IntervalOrDefault(def time.Duration) time.Duration {
	if a.Interval == nil || a.Interval.Duration <= 0 {
		return def
	}

	return a.Interval.Duration
}

func (a *Action) TimeoutOrDefault(def time.Duration) time.Duration {
	if a.Timeout == nil || a.Timeout.Duration <= 0 {
		return def
	}

	return a.Timeout.Duration
}
