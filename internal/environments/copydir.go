package environments

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyDir recursively copies the directory tree rooted at src into dst,
// creating dst (and its parents) if needed. Regular files and subdirectories
// are copied with their mode bits preserved; symlinks are recreated as
// symlinks (their targets are not followed). It's used to stage a fresh,
// throwaway copy of a test workspace's files for each sandbox launch, so the
// agent's edits never touch the source. A missing src is not an error — an
// empty test workspace stages as an empty directory.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return os.MkdirAll(dst, 0o755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", src)
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			mode := os.FileMode(0o755)
			if fi, err := d.Info(); err == nil {
				mode = fi.Mode().Perm()
			}
			return os.MkdirAll(target, mode)
		case d.Type()&os.ModeSymlink != 0:
			linkDest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(linkDest, target)
		default:
			return copyFile(path, target)
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
