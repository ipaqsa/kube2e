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

type Manager struct {
	templates map[string]*template.Template
	logger    *slog.Logger
}

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
		return fmt.Errorf("get blob from the '%s' dir: %w", dir, err)
	}

	for _, templateFile := range templates {
		templateName := strings.TrimSuffix(filepath.Base(templateFile), ".yaml")
		tmpl := template.New(templateName).Funcs(sprig.TxtFuncMap())

		content, err := os.ReadFile(templateFile)
		if err != nil {
			return fmt.Errorf("read the '%s' file: %w", templateFile, err)
		}

		m.logger.Debug("parse template", slog.String("name", templateName))

		tmpl, err = tmpl.Parse(string(content))
		if err != nil {
			return fmt.Errorf("parse the '%s' template: %w", templateFile, err)
		}

		m.templates[templateName] = tmpl
	}

	return nil
}

func (m *Manager) Render(name string, values map[string]interface{}) (client.Object, error) {
	m.logger.Debug("render template", slog.String("name", name))

	tmpl, ok := m.templates[name]
	if !ok {
		return nil, fmt.Errorf("the '%s' template not found", name)
	}

	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, values); err != nil {
		return nil, err
	}

	obj := new(unstructured.Unstructured)
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err := dec.Decode(buf.Bytes(), nil, obj); err != nil {
		return nil, fmt.Errorf("decode the '%s' template: %w", name, err)
	}

	return obj, nil
}
