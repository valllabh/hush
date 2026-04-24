package native

import "fmt"

// Tensor is a row-major float32 n-dimensional array. Methods prefer
// operating on existing storage (out-of-place) to simplify reasoning;
// hot paths can reuse an output tensor via SetData or fused helpers.
type Tensor struct {
	Shape []int
	Data  []float32
}

// NewTensor allocates a zero-filled tensor with the given shape.
func NewTensor(shape ...int) *Tensor {
	n := 1
	for _, d := range shape {
		n *= d
	}
	return &Tensor{Shape: append([]int(nil), shape...), Data: make([]float32, n)}
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

// Clone returns a copy.
func (t *Tensor) Clone() *Tensor {
	out := NewTensor(t.Shape...)
	copy(out.Data, t.Data)
	return out
}
