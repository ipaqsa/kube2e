package image

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

const (
	// testFileName is the descriptor file that marks a directory as a test suite.
	testFileName = "test.yaml"
)

// Build packages test directories from path into an image and pushes it to r.
func Build(ctx context.Context, r Remote, path string, logger *slog.Logger) error {
	ref, err := name.ParseReference(r.Ref)
	if err != nil {
		return fmt.Errorf("parse reference: %w", err)
	}

	dirs, err := findTestDirs(path)
	if err != nil {
		return fmt.Errorf("find test dirs: %w", err)
	}

	layerPath, err := writeTestLayer(path, dirs)
	if err != nil {
		return fmt.Errorf("write test layer: %w", err)
	}
	defer os.Remove(layerPath) //nolint:errcheck // best-effort cleanup for a temp file

	layer, err := tarball.LayerFromFile(layerPath)
	if err != nil {
		return fmt.Errorf("create layer: %w", err)
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return fmt.Errorf("append layer: %w", err)
	}

	logger.Info("push tests image", "image", r.Ref, "tests", len(dirs))

	if err = remote.Write(ref, img, registryOptions(ctx, r)...); err != nil {
		return fmt.Errorf("push image '%s': %w", r.Ref, err)
	}

	return nil
}

// findTestDirs returns immediate child directories that contain test.yaml.
func findTestDirs(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir '%s': %w", path, err)
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dir := filepath.Join(path, entry.Name())

		testFile := filepath.Join(dir, testFileName)
		if _, err = os.Stat(testFile); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, fmt.Errorf("stat test file '%s': %w", testFile, err)
		}

		dirs = append(dirs, dir)
	}

	if len(dirs) == 0 {
		return nil, fmt.Errorf("no test dirs found in '%s'", path)
	}

	return dirs, nil
}

// writeTestLayer writes a tar layer containing dirs at the image root.
func writeTestLayer(root string, dirs []string) (string, error) {
	f, err := os.CreateTemp("", "kube2e-tests-layer-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp layer: %w", err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(f.Name()) //nolint:errcheck // best-effort cleanup for a temp file
		}
	}()

	tw := tar.NewWriter(f)
	for _, dir := range dirs {
		if err = addDirToTar(tw, root, dir); err != nil {
			_ = tw.Close() //nolint:errcheck // preserving the original tar write error
			_ = f.Close()  //nolint:errcheck // preserving the original tar write error

			return "", err
		}
	}

	if err = tw.Close(); err != nil {
		_ = f.Close() //nolint:errcheck // preserving the original tar close error

		return "", fmt.Errorf("close tar: %w", err)
	}

	if err = f.Close(); err != nil {
		return "", fmt.Errorf("close layer: %w", err)
	}

	cleanup = false

	return f.Name(), nil
}

// addDirToTar appends dir and its contents to tw using paths relative to root.
func addDirToTar(tw *tar.Writer, root, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk '%s': %w", path, err)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat '%s': %w", path, err)
		}

		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read symlink '%s': %w", path, err)
			}
		}

		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return fmt.Errorf("create tar header '%s': %w", path, err)
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("build relative path '%s': %w", path, err)
		}

		hdr.Name = filepath.ToSlash(rel)
		if err = tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header '%s': %w", path, err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		if err = writeTarFile(tw, path); err != nil {
			return fmt.Errorf("write tar file '%s': %w", path, err)
		}

		return nil
	})
}

// writeTarFile streams path into tw.
func writeTarFile(tw *tar.Writer, path string) error {
	f, err := os.Open(path) //nolint:gosec // path comes from the requested tests directory
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file cleanup

	if _, err = io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}

	return nil
}

// registryOptions returns remote registry options for r.
func registryOptions(ctx context.Context, r Remote) []remote.Option {
	opts := []remote.Option{remote.WithContext(ctx)}

	if r.Username != "" {
		return append(opts, remote.WithAuth(&authn.Basic{
			Username: r.Username,
			Password: r.Password,
		}))
	}

	return append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
}
