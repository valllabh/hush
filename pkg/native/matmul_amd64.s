//go:build amd64

#include "textflag.h"

// func matmul4x4Kernel(a0, a1, a2, a3 *float32, bp *float32, K int,
//                      c0, c1, c2, c3 *float32)
//
// XMM (128-bit) lanes hold a single row of B (4 float32). VBROADCASTSS
// broadcasts A[r, k] into all 4 lanes. VFMADD231PS does the FMA.

TEXT ·matmul4x4Kernel(SB), NOSPLIT, $0-80
	MOVQ	a0+0(FP), AX
	MOVQ	a1+8(FP), BX
	MOVQ	a2+16(FP), CX
	MOVQ	a3+24(FP), DX
	MOVQ	bp+32(FP), SI
	MOVQ	K+40(FP), DI
	MOVQ	c0+48(FP), R8
	MOVQ	c1+56(FP), R9
	MOVQ	c2+64(FP), R10
	MOVQ	c3+72(FP), R11

	// Preload C tile into X4..X7 (accumulators).
	VMOVUPS	(R8), X4
	VMOVUPS	(R9), X5
	VMOVUPS	(R10), X6
	VMOVUPS	(R11), X7

	TESTQ	DI, DI
	JEQ	done

loop:
	// X0 = B_packed[k, 0..3]
	VMOVUPS	(SI), X0

	// Broadcast A[r, k] into X1..X3 / X8; FMA into the row accumulators.
	VBROADCASTSS	(AX), X1
	VFMADD231PS	X0, X1, X4

	VBROADCASTSS	(BX), X2
	VFMADD231PS	X0, X2, X5

	VBROADCASTSS	(CX), X3
	VFMADD231PS	X0, X3, X6

	VBROADCASTSS	(DX), X8
	VFMADD231PS	X0, X8, X7

	ADDQ	$4, AX
	ADDQ	$4, BX
	ADDQ	$4, CX
	ADDQ	$4, DX
	ADDQ	$16, SI
	DECQ	DI
	JNE	loop

done:
	VMOVUPS	X4, (R8)
	VMOVUPS	X5, (R9)
	VMOVUPS	X6, (R10)
	VMOVUPS	X7, (R11)
	VZEROUPPER
	RET
