// Package validate checks kube2e case files against the embedded case JSON Schema.
package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"sigs.k8s.io/yaml"

	"github.com/ipaqsa/kube2e/schemas"
)

const (
	// casesDirName is the subdirectory whose presence marks a directory as a test suite.
	casesDirName = "cases"
	// caseExt is the file extension of a case file.
	caseExt = ".yaml"
	// caseSchemaURL is the resource URL the embedded case schema is registered under.
	caseSchemaURL = "case.schema.json"
)

// ErrNoSuites is returned when a directory holds no test suites.
var ErrNoSuites = errors.New("no test suites found")

// printer localizes schema violation messages; kube2e reports them in English.
var printer = message.NewPrinter(language.English)

// pointerEscaper escapes the characters RFC 6901 reserves in a pointer token.
var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// Validator checks case documents against the kube2e case JSON Schema.
type Validator struct {
	schema *jsonschema.Schema
}

// New compiles the embedded case schema and returns a Validator.
func New() (*Validator, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.Case))
	if err != nil {
		return nil, fmt.Errorf("unmarshal case schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	// Registering the schema as a resource keeps compilation offline: its $id
	// is an https URL that the default loader would otherwise fetch.
	if err = compiler.AddResource(caseSchemaURL, document); err != nil {
		return nil, fmt.Errorf("add case schema: %w", err)
	}

	schema, err := compiler.Compile(caseSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile case schema: %w", err)
	}

	return &Validator{schema: schema}, nil
}

// Dir validates every case file of the test suites under dir. When dir itself
// holds a cases/ subdirectory it is validated as a single suite, otherwise its
// immediate child directories are. It reports ErrNoSuites when neither matches.
func (v *Validator) Dir(dir string) (*Report, error) {
	dirs, err := findSuiteDirs(dir)
	if err != nil {
		return nil, err
	}

	report := &Report{Dir: dir, Suites: make([]SuiteReport, 0, len(dirs))}

	for _, suiteDir := range dirs {
		suiteReport, suiteErr := v.suite(suiteDir)
		if suiteErr != nil {
			return nil, suiteErr
		}

		report.Suites = append(report.Suites, suiteReport)
	}

	return report, nil
}

// File validates a single case file. The returned error covers only reading the
// file; malformed YAML and schema violations are collected into the report.
func (v *Validator) File(path string) (CaseReport, error) {
	report := CaseReport{Path: path}

	content, err := os.ReadFile(path) //nolint:gosec // Case paths come from the directory the user asked to validate.
	if err != nil {
		return report, fmt.Errorf("read case '%s': %w", path, err)
	}

	document, err := decode(content)
	if err != nil {
		report.Problems = []Problem{{Pointer: rootPointer, Message: err.Error()}}

		return report, nil
	}

	if err = v.schema.Validate(document); err != nil {
		report.Problems = problems(err)
	}

	return report, nil
}

// suite validates every case file in the cases/ directory of suiteDir, in
// alphabetical filename order.
func (v *Validator) suite(suiteDir string) (SuiteReport, error) {
	report := SuiteReport{Name: filepath.Base(suiteDir), Path: suiteDir}
	casesDir := filepath.Join(suiteDir, casesDirName)

	entries, err := os.ReadDir(casesDir)
	if err != nil {
		return report, fmt.Errorf("read the cases dir '%s': %w", casesDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != caseExt {
			continue
		}

		caseReport, caseErr := v.File(filepath.Join(casesDir, entry.Name()))
		if caseErr != nil {
			return report, caseErr
		}

		report.Cases = append(report.Cases, caseReport)
	}

	return report, nil
}

// decode converts a YAML case document into the value model the validator expects.
func decode(content []byte) (any, error) {
	data, err := yaml.YAMLToJSON(content)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return document, nil
}

// findSuiteDirs returns the test suite directories under dir. A suite is a
// directory holding a cases/ subdirectory: dir itself when it has one,
// otherwise every immediate, non-hidden child directory that does.
func findSuiteDirs(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("check dir '%s': %w", dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", dir)
	}

	if isSuiteDir(dir) {
		return []string{dir}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir '%s': %w", dir, err)
	}

	dirs := make([]string, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if child := filepath.Join(dir, entry.Name()); isSuiteDir(child) {
			dirs = append(dirs, child)
		}
	}

	if len(dirs) == 0 {
		return nil, fmt.Errorf("%w in '%s'", ErrNoSuites, dir)
	}

	return dirs, nil
}

// isSuiteDir reports whether dir holds a cases/ subdirectory.
func isSuiteDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, casesDirName))

	return err == nil && info.IsDir()
}

// problems flattens a schema validation error into the violations it reports.
func problems(err error) []Problem {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return []Problem{{Pointer: rootPointer, Message: err.Error()}}
	}

	return collect(validationErr, rootPointer, nil)
}

// collect appends the most specific violations of err to into. Leaves carry the
// useful messages, because a parent that routes through a $ref only reports that
// its subschema failed. Location is the pointer of the nearest ancestor with a
// usable instance location and is reported for nodes that lack one themselves.
func collect(err *jsonschema.ValidationError, location string, into []Problem) []Problem {
	if reRoots(err) {
		return append(into, newProblem(err, location))
	}

	location = pointer(err.InstanceLocation)

	if len(err.Causes) == 0 {
		return append(into, newProblem(err, location))
	}

	for _, cause := range err.Causes {
		into = collect(cause, location, into)
	}

	return into
}

// reRoots reports whether the causes of err validate a different instance than
// err itself, as propertyNames does when it checks an object key on its own.
// Such a node is where jsonschema/v6 stores the validator's shared location
// buffer instead of a copy of it, so its own pointer has gone stale and the
// pointer of its nearest usable ancestor is reported instead.
func reRoots(err *jsonschema.ValidationError) bool {
	for _, cause := range err.Causes {
		if len(cause.InstanceLocation) < len(err.InstanceLocation) {
			return true
		}
	}

	return false
}

// newProblem converts a single schema violation at pointer into a Problem.
func newProblem(err *jsonschema.ValidationError, pointer string) Problem {
	return Problem{
		Pointer: pointer,
		Message: err.ErrorKind.LocalizedString(printer),
	}
}

// pointer renders an instance location as an RFC 6901 JSON pointer.
func pointer(location []string) string {
	if len(location) == 0 {
		return rootPointer
	}

	var builder strings.Builder

	for _, token := range location {
		builder.WriteByte('/')
		builder.WriteString(pointerEscaper.Replace(token))
	}

	return builder.String()
}
