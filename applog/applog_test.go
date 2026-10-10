package applog

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBufferedLogIsCompleteAfterClose(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	Init(p)
	g := Generation()
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				Logf("line %d", i)
			}
		}()
	}
	wg.Wait()
	if Generation() != g+16000 {
		t.Fatal("generation not advanced")
	}
	if len(Tail(300)) != 300 || len(Snapshot()) != maxBuf {
		t.Fatal("tail sizes")
	}
	Close()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "line "); n != 16000 {
		t.Fatalf("lines=%d", n)
	}
}
