package native

import "fmt"

// Tensor is a row-major float32 n-dimensional array. Methods prefer
// operating on existing storage (out-of-place) to simplify reasoning;
// hot paths can reuse an output tensor via SetData or fused helpers.
type Tensor struct {
	Shape []int
	Data  []float32

	// Packed, if non-nil, holds a pre-packed copy of Data in the layout
	// consumed by matmulPacked: for a [K, N] tensor this is
	//   [ceil(N/packNR), K, packNR]   (with a smaller tail panel)
	// Callers that use this tensor as the B matrix of a matmul should
	// prefer MatMulPacked when Packed != nil. Data is still kept for
	// correctness fallbacks and for tensors that are transposed/consumed
	// differently.
	Packed []float32
}

// NewTensor allocates a zero-filled tensor with the given shape.
func NewTensor(shape ...int) *Tensor {
	n := 1
	for _, d := range shape {
		n *= d
	}
	return &Tensor{Shape: append([]int(nil), shape...), Data: make([]float32, n)}
}

// arenaTensor returns a zero-filled tensor of the given shape, using the
// arena if non-nil, otherwise a fresh allocation. Ops in the forward path
// call this so they can run either standalone (tests, one-off MatMul) or
// inside an arena-scoped forward pass.
func arenaTensor(ar *Arena, shape ...int) *Tensor {
	if ar == nil {
		return NewTensor(shape...)
	}
	return ar.Get(shape...)
}

// FromSlice wraps an existing slice as a tensor of the given shape.
func FromSlice(shape []int, data []float32) *Tensor {
	n := 1
	for _, d := range shape {
		n *= d
	}
	if n != len(data) {
		panic(fmt.Sprintf("FromSlice: shape %v requires %d scalars, got %d", shape, n, len(data)))
	}
	return &Tensor{Shape: append([]int(nil), shape...), Data: data}
}

// Numel returns the total number of scalars in the tensor.
func (t *Tensor) Numel() int { return len(t.Data) }

// Stride returns the row-major stride at each axis.
func (t *Tensor) Stride() []int {
	strides := make([]int, len(t.Shape))
	s := 1
	for i := len(t.Shape) - 1; i >= 0; i-- {
		strides[i] = s
		s *= t.Shape[i]
	}
	return strides
}

// PackForMatMul pre-packs a 2D [K, N] tensor into the layout consumed by
// matmulPacked and stores it on t.Packed. Safe to call multiple times
// (re-packs). Panics if t is not 2D.
func (t *Tensor) PackForMatMul() *Tensor {
	if len(t.Shape) != 2 {
		panic(fmt.Sprintf("PackForMatMul needs 2D tensor, got %v", t.Shape))
	}
	K, N := t.Shape[0], t.Shape[1]
	t.Packed = packB(t.Data, K, N)
	return t
}

// Clone returns a copy.
func (t *Tensor) Clone() *Tensor {
	out := NewTensor(t.Shape...)
	copy(out.Data, t.Data)
	return out
}
