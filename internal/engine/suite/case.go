// Package suite parses and executes individual test cases.
package suite

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"sigs.k8s.io/yaml"

	"github.com/ipaqsa/kube2e/internal/engine/step"
)

const caseAnnotation = "testing.kube2e.io/case"

// caseValidator is a shared validator instance configured to use yaml field names.
var caseValidator = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("yaml")
		if name == "" || name == "-" {
			return fld.Name
		}

		return strings.SplitN(name, ",", 2)[0]
	})

	return v
}()

// Case holds the parsed contents of a case YAML file, including all steps to execute.
type Case struct {
	Path string `yaml:"path" json:"path"`

	// Version identifies the case schema version. Must equal "v1".
	Version string `yaml:"version" json:"version" validate:"required,eq=v1"`

	Name        string `yaml:"name"        json:"name"        validate:"required"`
	Description string `yaml:"description" json:"description"`

	// Tags are arbitrary labels used for filtering via --tags.
	// The case is skipped unless at least one of its tags matches the requested tags.
	Tags []string `yaml:"tags" json:"tags"`

	// Namespace is the Kubernetes namespace to create before the case runs if it
	// does not already exist. It is never deleted by kube2e, so a pre-existing
	// user namespace is left intact. Objects without an explicit namespace inherit
	// this value.
	Namespace string `yaml:"namespace" json:"namespace"`

	// Objects maps a resource name to its template base-filename (without .yaml).
	// The key becomes the Kubernetes object name injected into every render.
	// Steps reference an entry by name; Ensure is the only action that uses Values.
	Objects map[string]string `yaml:"objects" json:"objects"`

	Hooks Hooks        `yaml:"hooks"  json:"hooks"`
	Steps []*step.Step `yaml:"steps"  json:"steps"  validate:"dive"`
}

// Hooks holds case-level steps that run before and after every case step.
type Hooks struct {
	BeforeEach []*step.Step `yaml:"beforeEach" json:"beforeEach" validate:"dive"`
	AfterEach  []*step.Step `yaml:"afterEach"  json:"afterEach"  validate:"dive"`
}

// parseCaseFile reads and validates a case configuration from disk.
func parseCaseFile(path string) (*Case, error) {
	content, err := os.ReadFile(path) //nolint:gosec // path comes from trusted test configuration, not user input
	if err != nil {
		return nil, fmt.Errorf("read case file: %w", err)
	}

	tc := new(Case)
	if err = yaml.UnmarshalStrict(content, tc); err != nil {
		return nil, fmt.Errorf("unmarshal case file: %w", err)
	}

	if err = caseValidator.Struct(tc); err != nil {
		if verrs, ok := errors.AsType[validator.ValidationErrors](err); ok {
			return nil, fmt.Errorf("invalid case: %s", formatValidationErrors(verrs))
		}

		return nil, fmt.Errorf("validate case: %w", err)
	}

	tc.Path = path

	return tc, nil
}

// formatValidationErrors converts validation errors into a human-readable string
// using yaml field names that match what the user writes in their case files.
func formatValidationErrors(errs validator.ValidationErrors) string {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		// Drop the embedded actionOptions segment so paths read target.object.
		field := strings.ReplaceAll(strings.TrimPrefix(e.Namespace(), "Case."), "actionOptions.", "")

		var msg string

		switch e.Tag() {
		case "required":
			msg = field + ": required"
		case "required_without":
			msg = field + ": required when " + e.Param() + " is not set"
		case "required_with":
			msg = field + ": required when " + e.Param() + " is set"
		case "excluded_with":
			msg = field + ": must not be set together with " + e.Param()
		case "eq":
			msg = field + ": must equal " + e.Param()
		case "min":
			msg = field + ": must have at least " + e.Param() + " item(s)"
		default:
			msg = field + ": " + e.Tag()
		}

		msgs = append(msgs, msg)
	}

	return strings.Join(msgs, "; ")
}
