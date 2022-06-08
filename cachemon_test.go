package cachemon

import (
	"context"
	"crypto/rand"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/estroz/fs-cachemon/internal"
)

func TestFileChan(t *testing.T) {
	t.Run("in memory", testFileChanInMemory)
}

func testFileChanInMemory(t *testing.T) {
	opts := &Options{
		MaxSizeBytes: 1000,
		Interval:     100 * time.Millisecond,
	}

	root := internal.NewConcurrentMapFS(fstest.MapFS{
		"a": &fstest.MapFile{
			Data: makeDataSize(t, 100),
		},
		"b": &fstest.MapFile{
			Data: makeDataSize(t, 100),
		},
		"c": &fstest.MapFile{
			Data: makeDataSize(t, 200),
		},
		"d": &fstest.MapFile{
			Data: makeDataSize(t, 400),
		},
		"e": &fstest.MapFile{
			Data: makeDataSize(t, 200),
		},
		"f": &fstest.MapFile{
			Data: makeDataSize(t, 1),
		},
	})
	expected := []string{"f", "a", "b", "c", "d", "e"}
	require.NoError(t, fstest.TestFS(root, expected...))

	cache := &Cache{root: root}

	for _, exp := range expected {
		require.NoError(t, cache.Put(exp))
	}
	require.NoError(t, fstest.TestFS(root, genExpected(expected...)...))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	fc, err := run(ctx, root, opts)
	require.NoError(t, err)

	// Try once, should get the earliest file.
	hasMore11 := fc.Next()
	require.NoError(t, fc.Err())
	require.True(t, hasMore11)
	result11 := fc.Get()
	require.Equal(t, &Result{"f"}, result11)

	// Do not delete and try again, should get the earliest file again.
	hasMore12 := fc.Next()
	require.NoError(t, fc.Err())
	require.True(t, hasMore12)
	result12 := fc.Get()
	require.Equal(t, &Result{"f"}, result12)

	root.Delete("f")
	require.NoError(t, cache.Delete("f"))
	require.NoError(t, fstest.TestFS(root, genExpected("a", "b", "c", "d", "e")...))

	root.Add("g", &fstest.MapFile{
		Data: makeDataSize(t, 50),
	})
	require.NoError(t, cache.Put("g"))
	root.Add("h", &fstest.MapFile{
		Data: makeDataSize(t, 51),
	})
	require.NoError(t, cache.Put("h"))
	require.NoError(t, fstest.TestFS(root, genExpected("a", "b", "c", "d", "e", "g", "h")...))

	hasMore21 := fc.Next()
	require.NoError(t, fc.Err())
	require.True(t, hasMore21)
	result21 := fc.Get()
	require.Equal(t, &Result{"a"}, result21)

	root.Delete("a")
	require.NoError(t, cache.Delete("a"))

	hasMore22 := fc.Next()
	require.NoError(t, fc.Err())
	require.True(t, hasMore22)
	result22 := fc.Get()
	require.Equal(t, &Result{"b"}, result22)

	cancel()

	// Ended the loop, should get no error and a false value eventually
	// since "h" may be returned by Get() shortly after cancel().
	var (
		hasMore3 = true
		result3  *Result
	)
	for i := 0; i < 100; i++ {
		hasMore3 = fc.Next()
		require.NoError(t, fc.Err())
		if !hasMore3 {
			result3 = fc.Get()
			break
		}
	}
	require.False(t, hasMore3)
	require.Empty(t, result3)
}

func makeDataSize(t *testing.T, n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

func genExpected(expected ...string) (out []string) {
	for _, exp := range expected {
		out = append(out, exp, exp+monSuffix)
	}
	return out
}
