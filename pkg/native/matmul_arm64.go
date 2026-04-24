//go:build arm64

package native

import "unsafe"

// matmul4x4Kernel computes a 4-rows-of-A x 4-cols-of-B register tile:
//
//	for k in 0..K: C[r, 0..3] += A[r, k] * B_packed[k, 0..3]
//
// where B_packed is laid out as [K, 4] contiguous and each cRow* points at
// the 4 floats in C that receive the tile's accumulators. aRow* points at
// the first of K contiguous floats for that A row.
//
// Implemented in matmul_arm64.s using NEON (4x float32 lanes + FMLA).
//
//go:noescape
func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int, c0, c1, c2, c3 *float32)

// matmulPackedInner4x4 is the ASM-backed dispatch for the hot 4x4 tile.
// It is called from matmulPacked. Keeping a thin wrapper lets us swap
// kernels per-arch without touching the caller.
func matmulPackedInner4x4(a0, a1, a2, a3 []float32, bp []float32, K int, c0, c1, c2, c3 []float32) {
	// K==0 is a no-op; guard to avoid passing nil pointers from empty slices.
	if K == 0 {
		return
	}
	matmul4x4Kernel(
		(*float32)(unsafe.Pointer(&a0[0])),
		(*float32)(unsafe.Pointer(&a1[0])),
		(*float32)(unsafe.Pointer(&a2[0])),
		(*float32)(unsafe.Pointer(&a3[0])),
		(*float32)(unsafe.Pointer(&bp[0])),
		K,
		(*float32)(unsafe.Pointer(&c0[0])),
		(*float32)(unsafe.Pointer(&c1[0])),
		(*float32)(unsafe.Pointer(&c2[0])),
		(*float32)(unsafe.Pointer(&c3[0])),
	)
}
