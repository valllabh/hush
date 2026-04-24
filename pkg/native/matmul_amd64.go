//go:build amd64

package native

import "unsafe"

// matmul4x4Kernel: AVX/FMA 4x4 tile, same semantics as the arm64 version.
// B_packed is [K, 4] contiguous; C tiles are the 4 floats per row that
// receive the accumulated tile. Uses XMM (128-bit) lanes so the B-pack
// layout stays [N/4, K, 4] across all arches.
//
//go:noescape
func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int, c0, c1, c2, c3 *float32)

func matmulPackedInner4x4(a0, a1, a2, a3 []float32, bp []float32, K int, c0, c1, c2, c3 []float32) {
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
