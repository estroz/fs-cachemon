package internal

import (
	"io/fs"
	"sync"
	"testing/fstest"
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
	_ fs.StatFS     = (*ConcurrentMapFS)(nil)
	_ fs.ReadDirFS  = (*ConcurrentMapFS)(nil)
	_ fs.ReadFileFS = (*ConcurrentMapFS)(nil)
	_ fs.GlobFS     = (*ConcurrentMapFS)(nil)
	_ fs.SubFS      = (*ConcurrentMapFS)(nil)
)

func (cm *ConcurrentMapFS) Open(name string) (fs.File, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Open(name)
}

func (cm *ConcurrentMapFS) ReadFile(name string) ([]byte, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.ReadFile(name)
}

func (cm *ConcurrentMapFS) Stat(name string) (fs.FileInfo, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Stat(name)
}

func (cm *ConcurrentMapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.ReadDir(name)
}

func (cm *ConcurrentMapFS) Glob(pattern string) ([]string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Glob(pattern)
}

func (cm *ConcurrentMapFS) Sub(dir string) (fs.FS, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.mfs.Sub(dir)
}
