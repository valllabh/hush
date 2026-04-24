//go:build !arm64 && !amd64

package native

// Pure-Go fallback of the 4x4 packed kernel for architectures without a
// hand-written SIMD kernel (e.g. riscv64). Mirrors the register-tile body
// of matmulPacked's fast path.
func matmulPackedPanels(a0, a1, a2, a3 []float32, bPacked []float32, K, panels int, c0, c1, c2, c3 []float32) {
	const nr = 4
	for p := 0; p < panels; p++ {
		bp := bPacked[p*K*nr : (p+1)*K*nr]
		j := p * nr
		matmulPackedInner4x4(a0, a1, a2, a3, bp, K, c0[j:j+nr], c1[j:j+nr], c2[j:j+nr], c3[j:j+nr])
	}
}

func matmulPackedInner4x4(a0, a1, a2, a3 []float32, bp []float32, K int, c0, c1, c2, c3 []float32) {
	var r00, r01, r02, r03 float32
	var r10, r11, r12, r13 float32
	var r20, r21, r22, r23 float32
	var r30, r31, r32, r33 float32
	for k := 0; k < K; k++ {
		v0 := a0[k]
		v1 := a1[k]
		v2 := a2[k]
		v3 := a3[k]
		bOff := k * 4
		b0 := bp[bOff]
		b1 := bp[bOff+1]
		b2 := bp[bOff+2]
		b3 := bp[bOff+3]
		r00 += v0 * b0
		r01 += v0 * b1
		r02 += v0 * b2
		r03 += v0 * b3
		r10 += v1 * b0
		r11 += v1 * b1
		r12 += v1 * b2
		r13 += v1 * b3
		r20 += v2 * b0
		r21 += v2 * b1
		r22 += v2 * b2
		r23 += v2 * b3
		r30 += v3 * b0
		r31 += v3 * b1
		r32 += v3 * b2
		r33 += v3 * b3
	}
	c0[0] += r00
	c0[1] += r01
	c0[2] += r02
	c0[3] += r03
	c1[0] += r10
	c1[1] += r11
	c1[2] += r12
	c1[3] += r13
	c2[0] += r20
	c2[1] += r21
	c2[2] += r22
	c2[3] += r23
	c3[0] += r30
	c3[1] += r31
	c3[2] += r32
	c3[3] += r33
}
