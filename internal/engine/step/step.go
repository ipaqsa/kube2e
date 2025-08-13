// Package step orchestrates action execution within test steps.
package step

import (
	"fmt"

	"github.com/ipaqsa/kube2e/internal/engine/action"
)

const (
	stepAnnotation = "testing.kube2e.io/step"
)

// Step defines a single unit of work within a test case. All actions in a step
// operate on the same Kubernetes object identified by Object.
type Step struct {
	// Name is a required identifier shown in log output.
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	// Optional suppresses errors from this step so execution continues.
	Optional bool `yaml:"optional" json:"optional"`

	// Object is the resource name — a key in the case-level Objects map.
	// It becomes the Kubernetes object name injected into the template render.
	Object string `yaml:"object" json:"object"`

	Actions []action.Action `yaml:"actions" json:"actions"`

	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
}

// forEach executes the provided function for every action in the step.
func (s *Step) forEach(f func(act action.Action) error) error {
	for _, act := range s.Actions {
		if err := f(act); err != nil {
			return fmt.Errorf("perform the action '%s': %w", act.String(s.Object), err)
		}
	}

	return nil
}
