package test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"

	"kube2e/internal/engine/test"
)

type Manager struct {
	logger *slog.Logger
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger.With("manager", "test")}
}

type Config struct {
	Kubeconfig   string
	TestDir      string
	SpecificTest string
	SpecificCase string
}

func (m *Manager) Run(ctx context.Context, cfg *Config) error {
	restConfig, err := clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config from flags: %w", err)
	}

	tests, err := os.ReadDir(cfg.TestDir)
	if err != nil {
		return err
	}

	for _, testDir := range tests {
		if !testDir.IsDir() {
			continue
		}

		if len(cfg.SpecificTest) != 0 && cfg.SpecificTest != testDir.Name() {
			m.logger.Debug("skip test", "name", testDir.Name())
			continue
		}

		m.logger.Info("run test", slog.String("name", testDir.Name()))

		dir := filepath.Join(cfg.TestDir, testDir.Name())
		if err = test.Run(ctx, restConfig, dir, cfg.SpecificCase, m.logger); err != nil {
			return fmt.Errorf("run tests from the '%s' dir: %v", cfg.TestDir, err)
		}
	}

	return nil
}
