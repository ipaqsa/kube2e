package patch

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Patches []Patch

type Patch struct {
	Op    string      `json:"op"             yaml:"op"`
	Path  string      `json:"path"           yaml:"path"`
	From  string      `json:"from,omitempty" yaml:"from,omitempty"`
	Value interface{} `json:"value,omitempty" yaml:"value,omitempty"`
}

func (p *Patches) Apply(obj client.Object, opts *jsonpatch.ApplyOptions) (client.Object, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil object")
	}

	if opts == nil {
		opts = jsonpatch.NewApplyOptions()
	}

	marshalledObj, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	marshalledPatches, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal patches: %w", err)
	}

	parsedPath, err := jsonpatch.DecodePatch(marshalledPatches)
	if err != nil {
		return nil, fmt.Errorf("parse patches: %w", err)
	}

	patchedObj, err := parsedPath.ApplyWithOptions(marshalledObj, opts)
	if err != nil {
		return nil, fmt.Errorf("apply patches: %w", err)
	}

	parsedObj := new(unstructured.Unstructured)
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err = dec.Decode(patchedObj, nil, parsedObj); err != nil {
		return nil, fmt.Errorf("decode the '%s' object: %w", obj.GetName(), err)
	}

	return parsedObj, nil
}
