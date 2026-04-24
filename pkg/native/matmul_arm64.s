//go:build arm64

#include "textflag.h"

// func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int,
//                      c0, c1, c2, c3 *float32)
//
// Computes, across k = 0..K-1:
//     C_row_r[0..3] += A_row_r[k] * B_packed[k*4 + 0..3]
// for r in 0..3. The C tile is preloaded, accumulated, and written back.
//
// Registers:
//   R0..R3  = a0..a3 (advanced by 4 per k)
//   R4      = bp     (advanced by 16 per k)
//   R5      = K      loop counter
//   R6..R9  = c0..c3
//   V0      = B row (4 x float32)
//   V1..V4  = A broadcast (all lanes = A[r, k])
//   V16..V19 = C row accumulators

TEXT ·matmul4x4Kernel(SB), NOSPLIT, $0-80
	MOVD	a0+0(FP), R0
	MOVD	a1+8(FP), R1
	MOVD	a2+16(FP), R2
	MOVD	a3+24(FP), R3
	MOVD	bp+32(FP), R4
	MOVD	K+40(FP), R5
	MOVD	c0+48(FP), R6
	MOVD	c1+56(FP), R7
	MOVD	c2+64(FP), R8
	MOVD	c3+72(FP), R9

	// Preload existing C tile.
	VLD1	(R6), [V16.S4]
	VLD1	(R7), [V17.S4]
	VLD1	(R8), [V18.S4]
	VLD1	(R9), [V19.S4]

	CBZ	R5, done

loop:
	// V0 = B_packed[k, 0..3], post-increment bp by 16 bytes.
	VLD1.P	16(R4), [V0.S4]

	// Broadcast-load A[r, k] into all 4 lanes, post-increment by 4 bytes,
	// then FMLA. VFMLA <src1>, <src2>, <dst> computes dst += src1 * src2.
	VLD1R.P	4(R0), [V1.S4]
	VFMLA	V0.S4, V1.S4, V16.S4

	VLD1R.P	4(R1), [V2.S4]
	VFMLA	V0.S4, V2.S4, V17.S4

	VLD1R.P	4(R2), [V3.S4]
	VFMLA	V0.S4, V3.S4, V18.S4

	VLD1R.P	4(R3), [V4.S4]
	VFMLA	V0.S4, V4.S4, V19.S4

	SUBS	$1, R5, R5
	BNE	loop

done:
	VST1	[V16.S4], (R6)
	VST1	[V17.S4], (R7)
	VST1	[V18.S4], (R8)
	VST1	[V19.S4], (R9)
	RET
