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

		if err = extractEntry(root, dir, hdr, tr); err != nil {
			return err
		}
	}

	return nil
}

// extractEntry writes a single tar entry into dir, skipping entries whose
// resolved path (or, for symlinks, link target) would escape the extraction
// root.
func extractEntry(root, dir string, hdr *tar.Header, tr io.Reader) error {
	target := filepath.Join(dir, filepath.Clean("/"+hdr.Name))
	// Zip-slip guard: resolved path must stay inside dir.
	if !withinRoot(root, target) {
		return nil
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, 0o750); err != nil {
			return fmt.Errorf("mkdir '%s': %w", target, err)
		}
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("mkdir parent '%s': %w", target, err)
		}

		// Defense against tar-slip through a symlinked parent: the real,
		// symlink-resolved parent must still live inside the extraction root.
		if !realParentWithinRoot(root, target) {
			return nil
		}

		if err := writeFile(target, tr, hdr.FileInfo().Mode()); err != nil {
			return fmt.Errorf("write '%s': %w", target, err)
		}
	case tar.TypeSymlink:
		// Reject links whose resolved target escapes the extraction root,
		// so a later entry cannot be written through them to the host FS.
		if !symlinkWithinRoot(root, dir, target, hdr.Linkname) {
			return nil
		}

		if err := os.Symlink(hdr.Linkname, target); err != nil && !os.IsExist(err) {
			return fmt.Errorf("symlink '%s': %w", target, err)
		}
	}

	return nil
}

// withinRoot reports whether path stays inside root (root must end with a
// trailing separator).
func withinRoot(root, path string) bool {
	return strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), root)
}

// realParentWithinRoot resolves any symlinks in target's parent directory and
// reports whether the resolved parent is still inside root.
func realParentWithinRoot(root, target string) bool {
	realPath, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return false
	}

	return withinRoot(root, realPath)
}

// symlinkWithinRoot reports whether a symlink at target pointing to linkname
// resolves to a location inside root. Absolute targets are resolved relative to
// the extraction dir; relative targets relative to the link's own directory.
func symlinkWithinRoot(root, dir, target, linkname string) bool {
	var resolved string
	if filepath.IsAbs(linkname) {
		resolved = filepath.Join(dir, filepath.Clean("/"+linkname))
	} else {
		resolved = filepath.Join(filepath.Dir(target), linkname)
	}

	return withinRoot(root, resolved)
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
