package crd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	svckube "kube2e/internal/service/kube"
)

type Service struct {
	kube   *svckube.Service
	parser *parser
	crds   []*apiextensionv1.CustomResourceDefinition
	logger *slog.Logger
}

func New(kube *svckube.Service, crdDir string, logger *slog.Logger) (*Service, error) {
	svc := &Service{
		kube:   kube,
		parser: newParser(),
		crds:   make([]*apiextensionv1.CustomResourceDefinition, 0),
		logger: logger.With("service", "crd"),
	}

	svc.logger.Debug("parse crds")
	if err := svc.parseObjects(filepath.Join(crdDir, "*.yaml")); err != nil {
		return nil, fmt.Errorf("parse objects: %w", err)
	}

	svc.logger.Debug("service initialized")

	return svc, nil
}

func (s *Service) parseObjects(dir string) error {
	tmp, err := filepath.Glob(dir)
	if err != nil {
		return fmt.Errorf("get glob from the '%s' dir: %w", dir, err)
	}

	for _, file := range tmp {
		if strings.Contains(file, "doc-") {
			continue
		}

		s.logger.Debug("parse file", slog.String("name", file))

		parsed, err := s.parser.processFile(file)
		if err != nil {
			return fmt.Errorf("process the '%s' file: %w", file, err)
		}

		s.crds = append(s.crds, parsed...)
	}

	return nil
}

func (s *Service) GetCRDs() []*apiextensionv1.CustomResourceDefinition {
	return s.crds
}

func (s *Service) Ensure(ctx context.Context) error {
	for _, crd := range s.crds {
		s.logger.Debug("ensure crd", slog.String("name", crd.Name))
		if err := s.kube.Ensure(ctx, crd, svckube.WithEnsureToCache(false)); err != nil {
			return fmt.Errorf("ensure the '%s' crd: %w", crd.Name, err)
		}
	}

	return nil
}

func (s *Service) Delete(ctx context.Context) error {
	for _, crd := range s.crds {
		s.logger.Debug("delete crd", slog.String("name", crd.Name))
		if err := s.kube.Delete(ctx, crd); err != nil {
			return fmt.Errorf("delete the '%s' crd: %w", crd.Name, err)
		}
	}

	return nil
}
