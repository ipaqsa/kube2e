package step

import (
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"kube2e/internal/engine/action"
)

type Step struct {
	Path        string          `yaml:"path" json:"path"`
	Name        string          `yaml:"name" json:"name"`
	Description string          `yaml:"description" json:"description"`
	Actions     []action.Action `yaml:"actions" json:"actions"`
}

func parseStepFile(stepFile string) (*Step, error) {
	content, err := os.ReadFile(stepFile)
	if err != nil {
		return nil, fmt.Errorf("read the '%s' step file: %w", stepFile, err)
	}

	step := new(Step)
	if err = yaml.Unmarshal(content, step); err != nil {
		return nil, fmt.Errorf("unmarshal the '%s' step file: %w", stepFile, err)
	}

	if step.Name == "" {
		return nil, errors.New("step has no name")
	}

	step.Path = stepFile

	return step, nil
}

func (s *Step) forEach(f func(act action.Action) error) error {
	for _, act := range s.Actions {
		if err := f(act); err != nil {
			return fmt.Errorf("perform the '%s' action: %w", act.String(), err)
		}
	}

	return nil
}
