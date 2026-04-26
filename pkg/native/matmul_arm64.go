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
// Reachable on arm64 only via matmulPackedInner4x4; the linter does not
// trace through the ASM linkage so silence the noise.
//
//go:noescape
//nolint:unused
func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int, c0, c1, c2, c3 *float32)

// matmul4xNPanels runs the 4x4 kernel across `panels` contiguous B panels
// in a single ASM call so the Go->ASM call overhead is paid once per
// row-block rather than once per panel.
//
//go:noescape
func matmul4xNPanels(a0, a1, a2, a3 *float32, bp *float32, K, panels int, c0, c1, c2, c3 *float32)

// matmulPackedPanels is the ASM-backed multi-panel dispatch. It expects
// the full bPacked slice starting at panel 0 and the full C rows starting
// at column 0. It accumulates into c0..c3[0 : panels*4].
func matmulPackedPanels(a0, a1, a2, a3 []float32, bPacked []float32, K, panels int, c0, c1, c2, c3 []float32) {
	if K == 0 || panels == 0 {
		return
	}
	matmul4xNPanels(
		(*float32)(unsafe.Pointer(&a0[0])),
		(*float32)(unsafe.Pointer(&a1[0])),
		(*float32)(unsafe.Pointer(&a2[0])),
		(*float32)(unsafe.Pointer(&a3[0])),
		(*float32)(unsafe.Pointer(&bPacked[0])),
		K, panels,
		(*float32)(unsafe.Pointer(&c0[0])),
		(*float32)(unsafe.Pointer(&c1[0])),
		(*float32)(unsafe.Pointer(&c2[0])),
		(*float32)(unsafe.Pointer(&c3[0])),
	)
}

// matmulPackedInner4x4 is the per-panel dispatch for the hot 4x4 tile.
// On arm64 the live path is matmulPackedPanels (one ASM call per row
// block); this single-panel variant is kept for symmetry with the
// amd64 implementation and as a reference. Tagged unused so the linter
// stops flagging the unreachable arm64 branch.
//
//nolint:unused
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
