// Package template loads and renders Go templates backed by Sprig functions
// for Kubernetes manifest generation.
package template

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager holds pre-parsed Go templates keyed by their base filename (without
// the .yaml extension) and renders them on demand.
type Manager struct {
	templates map[string]*template.Template
	logger    *slog.Logger
}

// NewManager scans dir for *.yaml files, parses each as a Go template with
// Sprig helper functions, and returns a ready-to-use Manager.
func NewManager(dir string, logger *slog.Logger) (*Manager, error) {
	store := new(Manager)

	store.templates = make(map[string]*template.Template)
	store.logger = logger.With("manager", "template")

	store.logger.Debug("load templates")

	if err := store.parse(dir); err != nil {
		return nil, err
	}

	store.logger.Debug("manager initialized")

	return store, nil
}

func (m *Manager) parse(dir string) error {
	templates, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("get blob from the dir '%s': %w", dir, err)
	}

	for _, templateFile := range templates {
		templateName := strings.TrimSuffix(filepath.Base(templateFile), ".yaml")
		tmpl := template.New(templateName).Funcs(sprig.TxtFuncMap())

		content, err := os.ReadFile(templateFile) //nolint:gosec // path comes from trusted template directory, not user input
		if err != nil {
			return fmt.Errorf("read the file '%s': %w", templateFile, err)
		}

		m.logger.Debug("parse template", "name", templateName)

		tmpl, err = tmpl.Parse(string(content))
		if err != nil {
			return fmt.Errorf("parse the template '%s': %w", templateFile, err)
		}

		m.templates[templateName] = tmpl
	}

	return nil
}

// Render executes the named template with values and decodes the output into a
// Kubernetes Unstructured object. Returns an error when the template is unknown,
// execution fails, or the YAML cannot be decoded.
func (m *Manager) Render(name string, values map[string]interface{}) (client.Object, error) {
	m.logger.Debug("render template", "name", name)

	tmpl, ok := m.templates[name]
	if !ok {
		return nil, fmt.Errorf("the template '%s' not found", name)
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, values); err != nil {
		return nil, err
	}

	obj := new(unstructured.Unstructured)

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	if _, _, err := dec.Decode(buf.Bytes(), nil, obj); err != nil {
		return nil, fmt.Errorf("decode the template '%s': %w", name, err)
	}

	return obj, nil
}
