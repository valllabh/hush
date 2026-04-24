//go:build arm64

#include "textflag.h"

// func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int,
//                      c0, c1, c2, c3 *float32)
//
// Single-panel 4x4 NEON kernel. See matmul4xNPanels for the multi-panel
// variant that amortizes the Go->ASM call overhead across all panels of
// a row-block.
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

	VLD1	(R6), [V16.S4]
	VLD1	(R7), [V17.S4]
	VLD1	(R8), [V18.S4]
	VLD1	(R9), [V19.S4]

	CBZ	R5, done

loop:
	VLD1.P	16(R4), [V0.S4]

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

// func matmul4xNPanels(a0, a1, a2, a3 *float32, bp *float32, K, panels int,
//                      c0, c1, c2, c3 *float32)
//
// Processes all `panels` 4x4 tiles for a single row-block in one call.
// bp points at the packed B base for panel 0; each panel occupies K*4
// float32 = K*16 bytes. C tile for panel p lives at c{r} + p*16 bytes.
//
// Registers:
//   R0..R3  = a0..a3 (reset to a base each panel)
//   R4      = bp base   (advanced by K*16 per panel)
//   R5      = K inner counter (reloaded each panel)
//   R10     = Kconst (constant K, preserved across panels)
//   R11     = panels remaining
//   R6..R9  = c0..c3 (advanced by 16 per panel)
//   R12..R15 = saved copies of a0..a3 base so we can rewind
//   R16     = scratch (B walker)

TEXT ·matmul4xNPanels(SB), NOSPLIT, $0-88
	MOVD	a0+0(FP), R0
	MOVD	a1+8(FP), R1
	MOVD	a2+16(FP), R2
	MOVD	a3+24(FP), R3
	MOVD	bp+32(FP), R4
	MOVD	K+40(FP), R10
	MOVD	panels+48(FP), R11
	MOVD	c0+56(FP), R6
	MOVD	c1+64(FP), R7
	MOVD	c2+72(FP), R8
	MOVD	c3+80(FP), R9

	MOVD	R0, R12
	MOVD	R1, R13
	MOVD	R2, R14
	MOVD	R3, R15

	CBZ	R11, pdone

ploop:
	VLD1	(R6), [V16.S4]
	VLD1	(R7), [V17.S4]
	VLD1	(R8), [V18.S4]
	VLD1	(R9), [V19.S4]

	// Reset A pointers to base for this panel.
	MOVD	R12, R0
	MOVD	R13, R1
	MOVD	R14, R2
	MOVD	R15, R3
	// Current panel's B walker = R4 (advanced post-loop). R16 = working bp.
	MOVD	R4, R16
	MOVD	R10, R5

	CBZ	R5, kdone

kloop:
	VLD1.P	16(R16), [V0.S4]

	VLD1R.P	4(R0), [V1.S4]
	VFMLA	V0.S4, V1.S4, V16.S4

	VLD1R.P	4(R1), [V2.S4]
	VFMLA	V0.S4, V2.S4, V17.S4

	VLD1R.P	4(R2), [V3.S4]
	VFMLA	V0.S4, V3.S4, V18.S4

	VLD1R.P	4(R3), [V4.S4]
	VFMLA	V0.S4, V4.S4, V19.S4

	SUBS	$1, R5, R5
	BNE	kloop

kdone:
	VST1	[V16.S4], (R6)
	VST1	[V17.S4], (R7)
	VST1	[V18.S4], (R8)
	VST1	[V19.S4], (R9)

	// Advance: bp += K*16; c{r} += 16; panels--
	MOVD	R10, R5          // reuse R5 as scratch: K
	LSL	$4, R5, R5       // R5 = K*16
	ADD	R5, R4, R4

	ADD	$16, R6, R6
	ADD	$16, R7, R7
	ADD	$16, R8, R8
	ADD	$16, R9, R9

	SUBS	$1, R11, R11
	BNE	ploop

pdone:
	RET
