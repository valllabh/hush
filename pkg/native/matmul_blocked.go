package native

// Register-tiled matmul: process 4 rows of A x B at once so the
// inner loop issues 4 independent FMAs per B element, hiding FP
// latency. Pure Go; relies on the Go compiler's autovectorizer for
// the inner j-loop on Apple Silicon.
//
// Layout is row-major: C[M,N] += A[M,K] * B[K,N]. C must be zeroed.
//
// We chose mr=4 empirically on Apple M4 Pro:
//   baseline ikj single-row ~45 ms/op
//   mr=4                    ~29 ms/op  (1.58x)
//   mr=8                    ~30 ms/op  (register pressure)
//   cache blocking K/N      slower (M is small here)

func matmulBlocked(aData, bData, cData []float32, M, K, N int) {
	const mr = 4
	i := 0
	for ; i+mr <= M; i += mr {
		a0 := aData[(i+0)*K : (i+0)*K+K]
		a1 := aData[(i+1)*K : (i+1)*K+K]
		a2 := aData[(i+2)*K : (i+2)*K+K]
		a3 := aData[(i+3)*K : (i+3)*K+K]
		c0 := cData[(i+0)*N : (i+0)*N+N]
		c1 := cData[(i+1)*N : (i+1)*N+N]
		c2 := cData[(i+2)*N : (i+2)*N+N]
		c3 := cData[(i+3)*N : (i+3)*N+N]
		for k := 0; k < K; k++ {
			v0 := a0[k]
			v1 := a1[k]
			v2 := a2[k]
			v3 := a3[k]
			bRow := bData[k*N : k*N+N]
			for j := 0; j < N; j++ {
				bv := bRow[j]
				c0[j] += v0 * bv
				c1[j] += v1 * bv
				c2[j] += v2 * bv
				c3[j] += v3 * bv
			}
		}
	}
	// Tail rows.
	for ; i < M; i++ {
		aRow := aData[i*K : i*K+K]
		cRow := cData[i*N : i*N+N]
		for k := 0; k < K; k++ {
			av := aRow[k]
			if av == 0 {
				continue
			}
			bRow := bData[k*N : k*N+N]
			for j := 0; j < N; j++ {
				cRow[j] += av * bRow[j]
			}
		}
	}
}
