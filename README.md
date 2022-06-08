# fs-cachemon

This is a Go library for detecting a size bound violation on a directory,
intended to be used as a simple filesystem cache monitor.

Files least recently changed (LRU) are returned by `Get()` when the size of
the monitor's root directory exceeds the specified limit;
these files should be deleted to be fully evicted from the cache.

## Example

```go
import (
	"context"
	"log"
	"os"

	cachemon "github.com/estroz/fs-cachemon"
)

func main() {
	// Cancel this context to stop the cache monitory.
	ctx, cancel := context.WithCancel(context.Background())

	// The monitor will watch this cache dir for size that exceeds the threshold below.
	cacheRoot := "/tmp/cache"

	cache := cachemon.NewCache(cacheRoot)

	opts := &cachemon.Options{
		// Begin returning files as soon as cacheRoot exceeds 1 GB in size.
		MaxSizeBytes: 1e9,
	}

	// Start the monitor.
	cacheMon, err := cachemon.Run(ctx, cacheRoot, opts)
	checkErr(err)

	// cacheMon.Next() blocks on no events, so run the monitor loop in a separate goroutine.
	go func() {
		for cacheMon.Next() {
			// Get() the most recent cache eviction.
			result := cacheMon.Get()

			if err := os.RemoveAll(result.FilePath); err != nil {
				log.Printf("error removing cached file: %v", err)
			}
			if err := cache.Delete(result.FilePath); err != nil {
				log.Printf("error deleting file from cache: %v", err)
			}
		}
	}()


	// Do other tasks that read to and write from the cache.

	checkErr(ioutil.WriteFile("foo", []byte("blah"), 0777))
	checkErr(cache.Put("foo"))

	if exists, _ := cache.Get("foo"); exists {
		log.Printf("foo exists")
	}
	if exists, _ := cache.Get("bar"); !exists {
		log.Printf("bar does not exist")
	}

	// Finish up work and make sure no errors occurred.
	cancel()
	if err := cacheMon.Err(); err != nil {
		log.Printf("cache monitor error: %v", err)
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
```
