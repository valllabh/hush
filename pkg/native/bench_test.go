package native

import (
	"os"
	"testing"
)

func BenchmarkForward(b *testing.B) {
	path := ""
	for _, p := range []string{"../../../models/model.hbin", "../../models/model.hbin"} {
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		b.Skip("model.hbin not present")
	}
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	bn, err := Read(f)
	if err != nil {
		b.Fatal(err)
	}
	m, err := LoadModel(bn)
	if err != nil {
		b.Fatal(err)
	}

	T := m.Meta.SeqLen
	ids := make([]int32, T)
	mask := make([]int32, T)
	ids[0], ids[1], ids[2], ids[3] = 0, 100, 200, 2
	for i := 0; i < 4; i++ {
		mask[i] = 1
	}
	for i := 4; i < T; i++ {
		ids[i] = 1
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Forward(ids, mask)
	}
}
