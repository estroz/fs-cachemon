package cachemon

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FS interface {
	fs.StatFS
	Create(name string) (fs.File, error)
	Chtimes(name string, mtime, atime time.Time) error
	RemoveAll(name string) error
}

var _ FS = &afs{}

type afs struct {
	root string
}

func (a *afs) Open(name string) (fs.File, error) {
	rootName, err := a.mkdirRootName(name)
	if err != nil {
		return nil, err
	}
	return os.Open(rootName)
}

func (a *afs) Create(name string) (fs.File, error) {
	rootName, err := a.mkdirRootName(name)
	if err != nil {
		return nil, err
	}
	return os.Create(rootName)
}

func (a *afs) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(a.rootName(name))
}

func (a *afs) Chtimes(name string, mtime, atime time.Time) error {
	return os.Chtimes(a.rootName(name), mtime, atime)
}

func (a *afs) RemoveAll(name string) error {
	return os.RemoveAll(a.rootName(name))
}

func (a *afs) rootName(name string) string {
	if strings.HasPrefix(name, a.root) {
		return filepath.Clean(name)
	}
	return filepath.Join(a.root, name)
}

func (a *afs) mkdirRootName(name string) (string, error) {
	const sep = string(filepath.Separator)

	suffix := strings.TrimPrefix(filepath.Clean(name), a.root+sep)
	if !strings.Contains(suffix, sep) {
		return suffix, nil
	}

	dir, _ := filepath.Split(suffix)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return "", err
	}

	return a.rootName(suffix), nil
}
