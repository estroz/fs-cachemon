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

type Options struct {
	Interval     time.Duration
	MaxSizeBytes uint64
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

	return run(ctx, &statFS{os.DirFS(rootDir)}, opts)
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

type statFS struct {
	fs.FS
}

func (sfs *statFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(sfs.FS, name) }

func run(ctx context.Context, root fs.StatFS, opts *Options) (*FileChan, error) {
	fc := &FileChan{}
	fc.root = root
	fc.maxSize = opts.MaxSizeBytes
	fc.doneCtx = ctx
	fc.ch = make(chan *Result)

	go fc.run(ctx)

	return fc, nil
}

type FileChan struct {
	root     fs.StatFS
	maxSize  uint64
	interval time.Duration

	state fileState

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

		if err := fc.rebuild(); err != nil {
			fc.err = err
			return
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

// TODO: consider using https://github.com/fsnotify/fsnotify
// TODO: build a min heap instead of flat structure.

func (fc *FileChan) rebuild() error {

	fc.state = fileState{}
	err := fs.WalkDir(fc.root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		fc.state[path] = &finfo{info: info}

		return nil
	})

	return err
}

type finfo struct {
	info fs.FileInfo
}

type fileState map[string]*finfo

func (s fileState) getTimeSortedAsc() []*finfo {
	finfos := make([]*finfo, len(s))

	i := 0
	for _, fi := range s {
		finfos[i] = fi
		i++
	}

	sort.Slice(finfos, func(i, j int) bool {
		return finfos[i].info.ModTime().Before(finfos[j].info.ModTime())
	})

	return finfos
}

func (s fileState) getCurrentSize() (uint64, error) {

	var size uint64
	for _, fi := range s {
		size += uint64(fi.info.Size())
	}

	return size, nil
}

func (fc *FileChan) getExpired() ([]*finfo, error) {
	size, err := fc.state.getCurrentSize()
	if err != nil {
		return nil, err
	}

	if size <= fc.maxSize {
		return nil, nil
	}

	finfosSorted := fc.state.getTimeSortedAsc()

	i := 0
	for ; i < len(finfosSorted); i++ {
		if size -= uint64(finfosSorted[i].info.Size()); size <= fc.maxSize {
			i++
			break
		}
	}

	finfosOut := finfosSorted[:i]

	return finfosOut, nil
}
