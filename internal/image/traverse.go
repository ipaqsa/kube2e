// Package image provides types for referencing remote container image registries.
package image

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	crv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Remote holds credentials and address for a remote container registry.
type Remote struct {
	Ref      string
	Username string
	Password string
}

// Traverse pulls the image described by r, extracts its unified filesystem into
// a temporary directory, calls fn with that directory, and removes the directory
// when fn returns.
func Traverse(ctx context.Context, r Remote, fn func(dir string) error) error {
	img, err := pull(ctx, r)
	if err != nil {
		return fmt.Errorf("pull image '%s': %w", r.Ref, err)
	}

	dir, err := os.MkdirTemp("", "kube2e-image-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // not need to verify it

	if err = extract(img, dir); err != nil {
		return fmt.Errorf("extract image '%s': %w", r.Ref, err)
	}

	return fn(dir)
}

// pull fetches the image using the credentials in r. When Username is empty
// the default keychain (~/.docker/config.json, env vars, etc.) is used.
func pull(ctx context.Context, r Remote) (crv1.Image, error) {
	ref, err := name.ParseReference(r.Ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	opts := []remote.Option{remote.WithContext(ctx)}

	if r.Username != "" {
		opts = append(opts, remote.WithAuth(&authn.Basic{
			Username: r.Username,
			Password: r.Password,
		}))
	} else {
		opts = append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	img, err := remote.Image(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}

	return img, nil
}

// extract writes the merged image filesystem (layers applied in order) into dir.
func extract(img crv1.Image, dir string) error {
	rc := mutate.Extract(img)
	defer rc.Close() //nolint:errcheck // not need to verify it

	root := filepath.Clean(dir) + string(os.PathSeparator)

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Whiteout files mark deletions in overlay layers; skip them.
		if strings.HasPrefix(filepath.Base(hdr.Name), ".wh.") {
			continue
		}

		target := filepath.Join(dir, filepath.Clean("/"+hdr.Name))
		// Zip-slip guard: resolved path must stay inside dir.
		if !strings.HasPrefix(target+string(os.PathSeparator), root) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("mkdir '%s': %w", target, err)
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("mkdir parent '%s': %w", target, err)
			}

			if err = writeFile(target, tr, hdr.FileInfo().Mode()); err != nil {
				return fmt.Errorf("write '%s': %w", target, err)
			}
		case tar.TypeSymlink:
			if err = os.Symlink(hdr.Linkname, target); err != nil && !os.IsExist(err) {
				return fmt.Errorf("symlink '%s': %w", target, err)
			}
		}
	}

	return nil
}

// writeFile creates a file at path and streams content from r into it.
func writeFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec // path comes from trusted directory
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // not need to verify it

	if _, err = io.Copy(f, r); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}
