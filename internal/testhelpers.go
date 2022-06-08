package internal

import (
	"errors"
	"io/fs"
	"os"
	"sync"
	"testing/fstest"
	"time"
)

func NewConcurrentMapFS(mfs fstest.MapFS) *ConcurrentMapFS {
	return &ConcurrentMapFS{
		mfs: mfs,
	}
}

type ConcurrentMapFS struct {
	mfs fstest.MapFS
	mu  sync.Mutex
}

func (cm *ConcurrentMapFS) Add(name string, info *fstest.MapFile) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.mfs[name] = info
}

func (cm *ConcurrentMapFS) Delete(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.mfs, name)
}

var (
	_ fs.StatFS = (*ConcurrentMapFS)(nil)
)

func (cm *ConcurrentMapFS) Open(name string) (fs.File, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Open(name)
}

func (cm *ConcurrentMapFS) Stat(name string) (fs.FileInfo, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Stat(name)
}

func (cm *ConcurrentMapFS) Chtimes(name string, mtime, _ time.Time) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	fi, ok := cm.mfs[name]
	if !ok {
		return &fs.PathError{Op: "chtimes", Path: name, Err: os.ErrNotExist}
	}
	fi.ModTime = mtime
	cm.mfs[name] = fi
	return nil
}

func (cm *ConcurrentMapFS) Create(name string) (fs.File, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, ok := cm.mfs[name]; ok {
		return nil, &fs.PathError{Op: "create", Path: name, Err: os.ErrExist}
	}
	now := time.Now()
	f := &fstest.MapFile{ModTime: now}
	cm.mfs[name] = f
	return &mapFSFile{name: name, mf: f, mfs: cm.mfs}, nil
}

type mapFSFile struct {
	name string
	mf   *fstest.MapFile
	mfs  fstest.MapFS
}

var nierr = errors.New("not implemented")

func (f *mapFSFile) Close() error               { return nil }
func (f *mapFSFile) Stat() (fs.FileInfo, error) { return f.mfs.Stat(f.name) }
func (f *mapFSFile) Read(b []byte) (int, error) { return 0, nierr }

func (cm *ConcurrentMapFS) RemoveAll(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.mfs, name)
	return nil
}
