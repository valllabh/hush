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

// ---------------------------------------------------------------------------
// Packed B kernel
//
// Pack B [K, N] into panels of nr consecutive columns laid out as
//   packed[p * K * nr + k * nr + jj]  where j = p*nr + jj
// i.e. for each panel of nr columns, K rows are stored contiguously with
// the nr elements of that row adjacent. This is the "GotoBLAS" B-pack
// layout that lets a 4xnr register kernel walk k with unit-stride loads
// of B rows.
//
// Inner kernel: mr=4 rows of A x nr columns of B. The j loop inside the
// kernel is fixed length nr so the compiler can fully unroll / vectorize
// it; the k loop is the reduction axis.

const packNR = 4

// PackB packs B [K, N] into [ceil(N/nr), K, nr] panel layout.
// Trailing tail of N%nr columns is packed as a smaller panel of width
// equal to the remainder; callers detect the tail via j bounds.
func packB(bData []float32, K, N int) []float32 {
	nr := packNR
	panels := N / nr
	tail := N - panels*nr
	total := panels*K*nr + tail*K
	out := make([]float32, total)
	// full panels
	for p := 0; p < panels; p++ {
		jBase := p * nr
		dstBase := p * K * nr
		for k := 0; k < K; k++ {
			srcOff := k*N + jBase
			dstOff := dstBase + k*nr
			copy(out[dstOff:dstOff+nr], bData[srcOff:srcOff+nr])
		}
	}
	// tail panel: width = tail, laid out [K, tail]
	if tail > 0 {
		dstBase := panels * K * nr
		jBase := panels * nr
		for k := 0; k < K; k++ {
			srcOff := k*N + jBase
			dstOff := dstBase + k*tail
			copy(out[dstOff:dstOff+tail], bData[srcOff:srcOff+tail])
		}
	}
	return out
}

// matmulPacked computes C[M,N] += A[M,K] * B_packed, where B_packed is
// the output of packB(bData, K, N). C must be zeroed on entry.
func matmulPacked(aData, bPacked, cData []float32, M, K, N int) {
	const mr = 4
	nr := packNR
	panels := N / nr
	tail := N - panels*nr

	// Full-row blocks of A.
	i := 0
	for ; i+mr <= M; i += mr {
		a0 := aData[(i+0)*K : (i+0)*K+K]
		a1 := aData[(i+1)*K : (i+1)*K+K]
		a2 := aData[(i+2)*K : (i+2)*K+K]
		a3 := aData[(i+3)*K : (i+3)*K+K]
		c0Row := cData[(i+0)*N : (i+0)*N+N]
		c1Row := cData[(i+1)*N : (i+1)*N+N]
		c2Row := cData[(i+2)*N : (i+2)*N+N]
		c3Row := cData[(i+3)*N : (i+3)*N+N]

		for p := 0; p < panels; p++ {
			jBase := p * nr
			bp := bPacked[p*K*nr : (p+1)*K*nr]
			// Hand-written SIMD kernel (NEON on arm64, AVX/FMA on amd64,
			// pure-Go fallback elsewhere). Accumulates into the 4-wide C
			// tile, preserving the += semantics required by callers.
			matmulPackedInner4x4(
				a0, a1, a2, a3,
				bp, K,
				c0Row[jBase:jBase+nr],
				c1Row[jBase:jBase+nr],
				c2Row[jBase:jBase+nr],
				c3Row[jBase:jBase+nr],
			)
		}
		// tail panel (width = tail < nr)
		if tail > 0 {
			jBase := panels * nr
			bp := bPacked[panels*K*nr:]
			for k := 0; k < K; k++ {
				v0 := a0[k]
				v1 := a1[k]
				v2 := a2[k]
				v3 := a3[k]
				bRow := bp[k*tail : k*tail+tail]
				for jj := 0; jj < tail; jj++ {
					bv := bRow[jj]
					c0Row[jBase+jj] += v0 * bv
					c1Row[jBase+jj] += v1 * bv
					c2Row[jBase+jj] += v2 * bv
					c3Row[jBase+jj] += v3 * bv
				}
			}
		}
	}
	// Tail rows.
	for ; i < M; i++ {
		aRow := aData[i*K : i*K+K]
		cRow := cData[i*N : i*N+N]
		for p := 0; p < panels; p++ {
			jBase := p * nr
			bp := bPacked[p*K*nr : (p+1)*K*nr]
			for k := 0; k < K; k++ {
				av := aRow[k]
				if av == 0 {
					continue
				}
				bRow := bp[k*nr : k*nr+nr]
				cSlice := cRow[jBase : jBase+nr]
				for jj := 0; jj < nr; jj++ {
					cSlice[jj] += av * bRow[jj]
				}
			}
		}
		if tail > 0 {
			jBase := panels * nr
			bp := bPacked[panels*K*nr:]
			for k := 0; k < K; k++ {
				av := aRow[k]
				if av == 0 {
					continue
				}
				bRow := bp[k*tail : k*tail+tail]
				for jj := 0; jj < tail; jj++ {
					cRow[jBase+jj] += av * bRow[jj]
				}
			}
		}
	}
}
