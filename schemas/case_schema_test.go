package schemas

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestCaseSchemaCompiles verifies that case.schema.json is a valid JSON Schema.
func TestCaseSchemaCompiles(t *testing.T) {
	t.Parallel()

	compileCaseSchema(t)
}

// TestCaseSchemaExamples verifies every checked-in example case against the schema.
func TestCaseSchemaExamples(t *testing.T) {
	t.Parallel()

	schema := compileCaseSchema(t)

	err := filepath.WalkDir("../examples", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || filepath.Ext(path) != ".yaml" || filepath.Base(filepath.Dir(path)) != "cases" {
			return nil
		}

		t.Run(path, func(t *testing.T) {
			require.NoError(t, validateYAML(schema, path))
		})

		return nil
	})
	require.NoError(t, err)
}

// TestCaseSchemaFixtures verifies explicitly valid and invalid case fixtures.
func TestCaseSchemaFixtures(t *testing.T) {
	t.Parallel()

	schema := compileCaseSchema(t)

	valid, err := filepath.Glob("testdata/valid/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, valid)

	for _, path := range valid {
		t.Run("valid/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateYAML(schema, path))
		})
	}

	invalid, err := filepath.Glob("testdata/invalid/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, invalid)

	for _, path := range invalid {
		t.Run("invalid/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			require.Error(t, validateYAML(schema, path))
		})
	}
}

// TestCaseSchemaFormats verifies the reusable Kubernetes and OCI format definitions.
func TestCaseSchemaFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		definition string
		valid      string
		invalid    string
	}{
		{name: "duration", definition: "kubernetesDuration", valid: "1h30m", invalid: "tomorrow"},
		{name: "resource name", definition: "kubernetesResourceName", valid: "webapp.example", invalid: "Invalid_Name"},
		{name: "namespace", definition: "kubernetesNamespace", valid: "kube2e-tests", invalid: "kube2e.tests"},
		{name: "OCI reference", definition: "ociReference", valid: "ghcr.io/ipaqsa/kube2e:v1", invalid: "GHCR.IO/ipaqsa/kube2e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			compiler := newCaseSchemaCompiler()
			schema, err := compiler.Compile("case.schema.json#/$defs/" + tt.definition)
			require.NoError(t, err)
			require.NoError(t, schema.Validate(tt.valid))
			require.Error(t, schema.Validate(tt.invalid))
		})
	}
}

// compileCaseSchema compiles the case schema and fails the current test on error.
func compileCaseSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	schema, err := newCaseSchemaCompiler().Compile("case.schema.json")
	require.NoError(t, err)

	return schema
}

// newCaseSchemaCompiler returns a compiler configured to assert supported formats.
func newCaseSchemaCompiler() *jsonschema.Compiler {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	return compiler
}

// validateYAML converts a YAML document to JSON and validates it against schema.
func validateYAML(schema *jsonschema.Schema, path string) error {
	content, err := os.ReadFile(path) //nolint:gosec // Test paths are repository-controlled fixtures.
	if err != nil {
		return err
	}

	content, err = yaml.YAMLToJSON(content)
	if err != nil {
		return err
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return err
	}

	return schema.Validate(document)
}
