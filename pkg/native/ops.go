package native

import (
	"fmt"
	"math"
)

// AddBias adds a 1D bias of shape [M] across the last axis of x.
// x: [..., M]; bias: [M]. Mutates x in place and returns it.
func AddBias(x *Tensor, bias []float32) *Tensor {
	M := len(bias)
	n := len(x.Data) / M
	for i := 0; i < n; i++ {
		off := i * M
		for j := 0; j < M; j++ {
			x.Data[off+j] += bias[j]
		}
	}
	return x
}

// Add element-wise with broadcasting only along leading dims. Shapes
// must match in the trailing axes, and b's shape must be a prefix of a's.
// For the common case both shapes equal.
func Add(a, b *Tensor) *Tensor { return addArena(nil, a, b) }

func addArena(ar *Arena, a, b *Tensor) *Tensor {
	if len(a.Data) != len(b.Data) {
		panic(fmt.Sprintf("Add shape mismatch: %v vs %v", a.Shape, b.Shape))
	}
	out := arenaTensor(ar, a.Shape...)
	for i, v := range a.Data {
		out.Data[i] = v + b.Data[i]
	}
	return out
}

// AddInPlace computes a += b (same shape). Returns a for chaining.
func AddInPlace(a, b *Tensor) *Tensor {
	for i, v := range b.Data {
		a.Data[i] += v
	}
	return a
}

// MatMul computes a 2D matmul: A [M,K] x B [K,N] -> out [M,N].
func MatMul(a, b *Tensor) *Tensor { return matMulArena(nil, a, b) }

func matMulArena(ar *Arena, a, b *Tensor) *Tensor {
	if len(a.Shape) != 2 || len(b.Shape) != 2 {
		panic(fmt.Sprintf("MatMul needs 2D tensors, got %v and %v", a.Shape, b.Shape))
	}
	M, K := a.Shape[0], a.Shape[1]
	K2, N := b.Shape[0], b.Shape[1]
	if K != K2 {
		panic(fmt.Sprintf("MatMul K mismatch: %d vs %d", K, K2))
	}
	out := arenaTensor(ar, M, N)
	if b.Packed != nil {
		matmulPacked(a.Data, b.Packed, out.Data, M, K, N)
	} else {
		matmulBlocked(a.Data, b.Data, out.Data, M, K, N)
	}
	return out
}

// BatchMatMul computes [B,M,K] x [B,K,N] -> [B,M,N].
func BatchMatMul(a, b *Tensor) *Tensor { return batchMatMulArena(nil, a, b) }

func batchMatMulArena(ar *Arena, a, b *Tensor) *Tensor {
	if len(a.Shape) != 3 || len(b.Shape) != 3 {
		panic(fmt.Sprintf("BatchMatMul needs 3D, got %v and %v", a.Shape, b.Shape))
	}
	B, M, K := a.Shape[0], a.Shape[1], a.Shape[2]
	B2, K2, N := b.Shape[0], b.Shape[1], b.Shape[2]
	if B != B2 || K != K2 {
		panic(fmt.Sprintf("BatchMatMul shape mismatch: %v vs %v", a.Shape, b.Shape))
	}
	out := arenaTensor(ar, B, M, N)
	for bi := 0; bi < B; bi++ {
		aOff := bi * M * K
		bOff := bi * K * N
		oOff := bi * M * N
		matmulBlocked(
			a.Data[aOff:aOff+M*K],
			b.Data[bOff:bOff+K*N],
			out.Data[oOff:oOff+M*N],
			M, K, N,
		)
	}
	return out
}

// LayerNorm applies x = (x - mean) / sqrt(var + eps) * gamma + beta
// over the last axis. Standard fp32 impl matching ONNX LayerNormalization.
func LayerNorm(x *Tensor, gamma, beta []float32, eps float32) *Tensor {
	return layerNormArena(nil, x, gamma, beta, eps)
}

func layerNormArena(ar *Arena, x *Tensor, gamma, beta []float32, eps float32) *Tensor {
	H := x.Shape[len(x.Shape)-1]
	if len(gamma) != H || len(beta) != H {
		panic(fmt.Sprintf("LayerNorm size mismatch: H=%d gamma=%d beta=%d", H, len(gamma), len(beta)))
	}
	n := len(x.Data) / H
	out := arenaTensor(ar, x.Shape...)
	for i := 0; i < n; i++ {
		off := i * H
		// mean
		var m float64
		for j := 0; j < H; j++ {
			m += float64(x.Data[off+j])
		}
		m /= float64(H)
		// variance
		var v float64
		for j := 0; j < H; j++ {
			d := float64(x.Data[off+j]) - m
			v += d * d
		}
		v /= float64(H)
		invStd := float32(1.0 / math.Sqrt(v+float64(eps)))
		for j := 0; j < H; j++ {
			out.Data[off+j] = (x.Data[off+j]-float32(m))*invStd*gamma[j] + beta[j]
		}
	}
	return out
}

// Softmax along the last axis.
func Softmax(x *Tensor) *Tensor { return softmaxArena(nil, x) }

func softmaxArena(ar *Arena, x *Tensor) *Tensor {
	H := x.Shape[len(x.Shape)-1]
	n := len(x.Data) / H
	out := arenaTensor(ar, x.Shape...)
	for i := 0; i < n; i++ {
		off := i * H
		// max for numerical stability
		maxv := x.Data[off]
		for j := 1; j < H; j++ {
			if v := x.Data[off+j]; v > maxv {
				maxv = v
			}
		}
		var sum float64
		for j := 0; j < H; j++ {
			e := math.Exp(float64(x.Data[off+j] - maxv))
			out.Data[off+j] = float32(e)
			sum += e
		}
		inv := float32(1.0 / sum)
		for j := 0; j < H; j++ {
			out.Data[off+j] *= inv
		}
	}
	return out
}

// GELU applies the exact GELU activation using math.Erf, matching the
// ONNX export path that uses Erf (not the tanh approximation).
//
//	GELU(x) = x * 0.5 * (1 + erf(x / sqrt(2)))
func GELU(x *Tensor) *Tensor { return geluArena(nil, x) }

func geluArena(ar *Arena, x *Tensor) *Tensor {
	const invSqrt2 = 0.7071067811865475
	out := arenaTensor(ar, x.Shape...)
	for i, v := range x.Data {
		out.Data[i] = v * 0.5 * (1 + float32(math.Erf(float64(v)*invSqrt2)))
	}
	return out
}

// Transpose rearranges axes. Supports arbitrary n-D tensors. General
// path is slow; callers wanting attention-shape transposes should use
// Transpose4D below.
func Transpose(t *Tensor, axes []int) *Tensor { return transposeArena(nil, t, axes) }

func transposeArena(ar *Arena, t *Tensor, axes []int) *Tensor {
	if len(axes) != len(t.Shape) {
		panic(fmt.Sprintf("Transpose axes %v vs shape %v", axes, t.Shape))
	}
	var newShape [8]int
	if len(axes) > len(newShape) {
		panic("transpose rank > 8 not supported")
	}
	ns := newShape[:len(axes)]
	for i, a := range axes {
		ns[i] = t.Shape[a]
	}
	out := arenaTensor(ar, ns...)
	inStrides := t.Stride()
	outStrides := out.Stride()
	rank := len(t.Shape)

	var rec func(depth, srcOff, dstOff int)
	rec = func(depth, srcOff, dstOff int) {
		if depth == rank {
			out.Data[dstOff] = t.Data[srcOff]
			return
		}
		for i := 0; i < newShape[depth]; i++ {
			rec(depth+1, srcOff+i*inStrides[axes[depth]], dstOff+i*outStrides[depth])
		}
	}
	rec(0, 0, 0)
	return out
}

// Reshape returns a tensor sharing t.Data with a new shape (no copy).
// Panics if the totals do not match.
func Reshape(t *Tensor, shape ...int) *Tensor { return reshapeArena(nil, t, shape...) }

// reshapeArena returns a view tensor sharing t.Data with a new shape.
// When ar is non-nil the returned Tensor struct and Shape slice come from
// the arena so the view is free of heap allocations.
func reshapeArena(ar *Arena, t *Tensor, shape ...int) *Tensor {
	n := 1
	for _, d := range shape {
		n *= d
	}
	if n != len(t.Data) {
		panic(fmt.Sprintf("Reshape totals mismatch: %v vs %v", t.Shape, shape))
	}
	if ar == nil {
		return &Tensor{Shape: append([]int(nil), shape...), Data: t.Data}
	}
	v := ar.view(shape)
	v.Data = t.Data
	return v
}

// Gather is the embedding lookup: given a [V, H] table and an index
// tensor of shape [B, T], returns a [B, T, H] tensor.
func Gather(table *Tensor, indices []int32, B, T int) *Tensor {
	return gatherArena(nil, table, indices, B, T)
}

func gatherArena(ar *Arena, table *Tensor, indices []int32, B, T int) *Tensor {
	H := table.Shape[1]
	V := table.Shape[0]
	out := arenaTensor(ar, B, T, H)
	for i := 0; i < B*T; i++ {
		idx := int(indices[i])
		if idx < 0 || idx >= V {
			// out of range; fill zeros and continue (behaviour matches onnx with negative idx handling elsewhere)
			continue
		}
		copy(out.Data[i*H:(i+1)*H], table.Data[idx*H:(idx+1)*H])
	}
	return out
}

// ScaleInPlace multiplies every element by s. Returns t.
func ScaleInPlace(t *Tensor, s float32) *Tensor {
	for i := range t.Data {
		t.Data[i] *= s
	}
	return t
}

// ApplyAdditiveMask adds a large negative value to entries of x where
// mask is 0, so that subsequent softmax zeroes them out. mask must be
// broadcast-compatible with x over trailing axes.
func ApplyAdditiveMask(x *Tensor, mask []float32) *Tensor {
	// mask length divides x.Data length; pattern repeats.
	m := len(mask)
	if len(x.Data)%m != 0 {
		panic(fmt.Sprintf("mask size %d does not divide x size %d", m, len(x.Data)))
	}
	for i := range x.Data {
		x.Data[i] += mask[i%m]
	}
	return x
}
