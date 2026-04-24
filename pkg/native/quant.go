package native

import "fmt"

// QuantWeight holds a per-output-channel symmetrically quantized int8
// weight matrix in [In, Out] layout along with fp32 scales of length
// [Out]. Dequantized value at position (k, j) is:
//
//	W[k, j] = int8Data[k*Out + j] * Scale[j]
//
// Zero point is 0 (symmetric quantization). Scales are always positive.
type QuantWeight struct {
	In, Out int
	Data    []int8    // length In*Out
	Scale   []float32 // length Out
}

// NewQuantWeight constructs a QuantWeight from raw buffers, validating
// shape consistency.
func NewQuantWeight(in, out int, data []int8, scale []float32) (*QuantWeight, error) {
	if len(data) != in*out {
		return nil, fmt.Errorf("int8 data len %d != in*out (%d*%d)", len(data), in, out)
	}
	if len(scale) != out {
		return nil, fmt.Errorf("scale len %d != out %d", len(scale), out)
	}
	return &QuantWeight{In: in, Out: out, Data: data, Scale: scale}, nil
}

// MatMulInt8 computes A [M, K] fp32 x W (int8 [K, N] + fp32 scale [N])
// -> out [M, N] fp32. Correct-first implementation: dequantize each K-row
// of W into an fp32 buffer once and feed into the standard blocked matmul.
// This keeps the hot inner loop identical to the fp32 path while paying
// the dequant cost only once per call (K*N multiplies).
//
// Activations stay fp32; no activation-side quantization.
func MatMulInt8(a *Tensor, w *QuantWeight) *Tensor {
	if len(a.Shape) != 2 {
		panic(fmt.Sprintf("MatMulInt8 needs 2D A, got %v", a.Shape))
	}
	M, K := a.Shape[0], a.Shape[1]
	if K != w.In {
		panic(fmt.Sprintf("MatMulInt8 K mismatch: A has %d, W has %d", K, w.In))
	}
	N := w.Out
	// Dequantize W into a fresh [K, N] fp32 buffer.
	deq := make([]float32, K*N)
	for k := 0; k < K; k++ {
		srcOff := k * N
		dstOff := k * N
		src := w.Data[srcOff : srcOff+N]
		dst := deq[dstOff : dstOff+N]
		for j := 0; j < N; j++ {
			dst[j] = float32(src[j]) * w.Scale[j]
		}
	}
	out := NewTensor(M, N)
	matmulBlocked(a.Data, deq, out.Data, M, K, N)
	return out
}

// MaybeWeight is a tagged union holding either a dense fp32 weight or a
// per-output-channel int8 quantized weight. Exactly one of F32 or I8 is
// non-nil. Helper MatMul dispatches to the right kernel.
type MaybeWeight struct {
	F32 *Tensor      // non-nil for fp32 weights
	I8  *QuantWeight // non-nil for int8 weights
}

// IsInt8 reports whether this is an int8 quantized weight.
func (w *MaybeWeight) IsInt8() bool { return w.I8 != nil }

// MatMul dispatches to fp32 MatMul or MatMulInt8 depending on storage.
// For fp32 weights, the caller must have already oriented the weight to
// [K, N] (matching MatMul semantics). For int8 weights, the stored
// layout is always [In, Out] == [K, N].
func (w *MaybeWeight) MatMul(a *Tensor) *Tensor {
	if w.I8 != nil {
		return MatMulInt8(a, w.I8)
	}
	return MatMul(a, w.F32)
}
