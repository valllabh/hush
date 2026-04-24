package native

import (
	"math"
	"os"
	"testing"
)

// loadModelForBench loads model.hbin if available; tests/benches skip otherwise.
func loadModelForBench(tb testing.TB) *Model {
	tb.Helper()
	path := ""
	for _, p := range []string{"../../../models/model.hbin", "../../models/model.hbin"} {
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		tb.Skip("model.hbin not present")
	}
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	bn, err := Read(f)
	if err != nil {
		tb.Fatal(err)
	}
	m, err := LoadModel(bn)
	if err != nil {
		tb.Fatal(err)
	}
	return m
}

// makeInputs builds B examples each of length usedLen (non-pad), padded
// to seqLen. Token ids cycle through a small set to keep lookups valid.
func makeInputs(m *Model, B, usedLen int) (ids, mask []int32) {
	T := m.Meta.SeqLen
	ids = make([]int32, B*T)
	mask = make([]int32, B*T)
	for b := 0; b < B; b++ {
		off := b * T
		// Token 0 CLS, then varied ids, then 2 EOS, then pad (1).
		ids[off] = 0
		for i := 1; i < usedLen-1; i++ {
			ids[off+i] = int32(100 + (b*17+i*13)%5000)
		}
		ids[off+usedLen-1] = 2
		for i := 0; i < usedLen; i++ {
			mask[off+i] = 1
		}
		for i := usedLen; i < T; i++ {
			ids[off+i] = 1 // pad
		}
	}
	return
}

// TestForwardBatchMatchesForward verifies ForwardBatch over B examples
// produces numerics equivalent to looping Forward per example.
func TestForwardBatchMatchesForward(t *testing.T) {
	m := loadModelForBench(t)
	T := m.Meta.SeqLen

	B := 3
	// Give each example a different effective length so trim logic works.
	lens := []int{12, 20, 8}
	ids := make([]int32, B*T)
	mask := make([]int32, B*T)
	for b := 0; b < B; b++ {
		L := lens[b]
		off := b * T
		ids[off] = 0
		for i := 1; i < L-1; i++ {
			ids[off+i] = int32(100 + (b*31+i*17)%5000)
		}
		ids[off+L-1] = 2
		for i := 0; i < L; i++ {
			mask[off+i] = 1
		}
		for i := L; i < T; i++ {
			ids[off+i] = 1
		}
	}

	got := m.ForwardBatch(ids, mask, B)
	if len(got) != B {
		t.Fatalf("ForwardBatch returned %d rows", len(got))
	}
	for b := 0; b < B; b++ {
		want := m.Forward(ids[b*T:(b+1)*T], mask[b*T:(b+1)*T])
		if len(want) != len(got[b]) {
			t.Fatalf("row %d: len mismatch %d vs %d", b, len(want), len(got[b]))
		}
		for i := range want {
			diff := math.Abs(float64(want[i] - got[b][i]))
			if diff > 1e-3 {
				t.Errorf("row %d logit %d: want %v got %v (diff %v)", b, i, want[i], got[b][i], diff)
			}
		}
	}
}

func benchBatch(b *testing.B, batch, usedLen int) {
	m := loadModelForBench(b)
	T := m.Meta.SeqLen
	if usedLen > T {
		usedLen = T
	}
	ids, mask := makeInputs(m, batch, usedLen)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.ForwardBatch(ids, mask, batch)
	}
}

func benchLoop(b *testing.B, batch, usedLen int) {
	m := loadModelForBench(b)
	T := m.Meta.SeqLen
	if usedLen > T {
		usedLen = T
	}
	ids, mask := makeInputs(m, batch, usedLen)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for e := 0; e < batch; e++ {
			_ = m.Forward(ids[e*T:(e+1)*T], mask[e*T:(e+1)*T])
		}
	}
}

// Realistic T=64 workload (matches the ~60 token hush input profile).
func BenchmarkBatchForward_1(b *testing.B)  { benchBatch(b, 1, 64) }
func BenchmarkBatchForward_2(b *testing.B)  { benchBatch(b, 2, 64) }
func BenchmarkBatchForward_4(b *testing.B)  { benchBatch(b, 4, 64) }
func BenchmarkBatchForward_10(b *testing.B) { benchBatch(b, 10, 64) }

// Looped single-example baseline at the same T for apples-to-apples.
func BenchmarkLoopForward_1(b *testing.B)  { benchLoop(b, 1, 64) }
func BenchmarkLoopForward_2(b *testing.B)  { benchLoop(b, 2, 64) }
func BenchmarkLoopForward_4(b *testing.B)  { benchLoop(b, 4, 64) }
func BenchmarkLoopForward_10(b *testing.B) { benchLoop(b, 10, 64) }
