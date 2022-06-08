package cachemon

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"
)

func NewCache(rootDir string) *Cache {
	return &Cache{
		root: &afs{
			root: rootDir,
		},
	}
}

type Cache struct {
	root FS
}

func (c *Cache) Put(filePath string) error {
	return c.updateMonFile(filePath)
}

func (c *Cache) Get(filePath string) (bool, error) {
	if _, err := c.root.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return true, c.updateMonFile(filePath)
}

func (c *Cache) Delete(filePath string) error {
	return c.root.RemoveAll(filePath + monSuffix)
}

const monSuffix = ".mon"

func (c *Cache) updateMonFile(filePath string) error {

	filePath += monSuffix
	now := time.Now()
	if cherr := c.root.Chtimes(filePath, now, now); cherr != nil {
		f, cerr := c.root.Create(filePath)
		if cerr != nil {
			return cerr
		}
		return f.Close()
	}

	return nil
}

type Options struct {
	Interval     time.Duration
	MaxSizeBytes int64
}

const (
	defaultMaxSize  = 50e9 // 50 GB
	defaultInterval = 3 * time.Second
)

func Run(ctx context.Context, rootDir string, opts *Options) (*FileChan, error) {

	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}

	if opts.MaxSizeBytes == 0 {
		opts.MaxSizeBytes = defaultMaxSize
	}

	if err := os.MkdirAll(rootDir, 0777); err != nil {
		return nil, err
	}

	return run(ctx, &afs{root: rootDir}, opts)
}

func RunBackground(ctx context.Context, rootDir string, opts *Options, f func(*Result)) error {

	cacheMon, err := Run(ctx, rootDir, opts)
	if err != nil {
		return err
	}

	go func() {
		for cacheMon.Next() {
			result := cacheMon.Get()

			f(result)
		}
	}()

	return nil
}

func run(ctx context.Context, root FS, opts *Options) (*FileChan, error) {
	fc := &FileChan{}
	fc.root = root
	fc.maxSize = opts.MaxSizeBytes
	fc.doneCtx = ctx
	fc.ch = make(chan *Result)

	go fc.run(ctx)

	return fc, nil
}

type FileChan struct {
	root     FS
	maxSize  int64
	interval time.Duration

	ch      chan *Result
	doneCtx context.Context
	err     error
	curr    *Result
}

type Result struct {
	FilePath string
}

func (fc *FileChan) Next() bool {
	for {
		select {
		case <-fc.doneCtx.Done():
			fc.curr = nil
			return false

		case r, ok := <-fc.ch:
			if !ok {
				fc.curr = nil
				return false
			}

			// Stat prior to returning so no files deleted between
			// rebuild() and Get() are returned.
			if _, err := fc.root.Stat(r.FilePath); err != nil && !errors.Is(err, os.ErrExist) {
				continue
			}

			fc.curr = r
			return true
		}
	}
}

func (fc *FileChan) Get() *Result {
	return fc.curr
}

func (fc *FileChan) Err() error {
	return fc.err
}

func (fc *FileChan) run(ctx context.Context) {
	defer close(fc.ch)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		expired, err := fc.getExpired()
		if err != nil {
			fc.err = err
			return
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, exp := range expired {
				fc.ch <- &Result{
					FilePath: exp.info.Name(),
				}
			}
		}()
		time.Sleep(fc.interval)
		wg.Wait()
	}
}

// TODO: build a min heap instead of flat structure.

func (fc *FileChan) getExpired() ([]*finfo, error) {

	var size int64
	finfos := []*finfo{}
	err := fs.WalkDir(fc.root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if monInfo, err := fc.root.Stat(path + monSuffix); err == nil || errors.Is(err, os.ErrExist) {
			info, err := d.Info()
			if err != nil {
				return err
			}

			size += info.Size()
			finfos = append(finfos, &finfo{
				info: info,
				mt:   monInfo.ModTime(),
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if size <= fc.maxSize {
		return nil, nil
	}

	sort.SliceStable(finfos, func(i, j int) bool {
		return finfos[i].mt.Before(finfos[j].mt)
	})

	i := 0
	for ; i < len(finfos); i++ {
		if size -= finfos[i].info.Size(); size <= fc.maxSize {
			i++
			break
		}
	}

	finfosOut := finfos[:i]

	return finfosOut, nil
}

type finfo struct {
	info fs.FileInfo
	mt   time.Time
}
