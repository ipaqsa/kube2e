package crd

import (
	"bytes"
	"fmt"
	"io"
	"os"

	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	crdKind = "CustomResourceDefinition"

	bufferSize = 1024 * 1024
)

type parser struct {
	buffer []byte
}

func newParser() *parser {
	return &parser{
		buffer: make([]byte, bufferSize),
	}
}

func (p *parser) processFile(crdPath string) ([]*apiextensionv1.CustomResourceDefinition, error) {
	if len(crdPath) == 0 {
		return nil, nil
	}

	file, err := os.Open(crdPath)
	if err != nil {
		return nil, fmt.Errorf("open the '%s' crd file: %w", crdPath, err)
	}
	defer file.Close()

	reader := utilyaml.NewDocumentDecoder(file)
	var crds []*apiextensionv1.CustomResourceDefinition
	for {
		n, err := reader.Read(p.buffer)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, err
		}

		data := p.buffer[:n]
		// some empty yaml document, or empty string before separator
		if len(data) == 0 {
			continue
		}

		crd, err := p.parseCRD(bytes.NewReader(data), n)
		if err != nil {
			return nil, err
		}
		if crd != nil {
			crds = append(crds, crd)
		}
	}

	return crds, nil
}

func (p *parser) parseCRD(reader io.Reader, bufferSize int) (*apiextensionv1.CustomResourceDefinition, error) {
	crd := new(apiextensionv1.CustomResourceDefinition)
	if err := utilyaml.NewYAMLOrJSONDecoder(reader, bufferSize).Decode(crd); err != nil {
		return nil, fmt.Errorf("parse crd: %w", err)
	}

	// it could be a comment or some other peace of yaml file, skip it
	if crd == nil {
		return nil, nil
	}

	if crd.APIVersion != apiextensionv1.SchemeGroupVersion.String() && crd.Kind != crdKind {
		return nil, fmt.Errorf("invalid CRD('%s/%s')", crd.APIVersion, crd.Kind)
	}

	return crd, nil
}
