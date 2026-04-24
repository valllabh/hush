package native

import (
	"fmt"
	"math"
)

// Layer holds all weights for one transformer block.
type Layer struct {
	QueryW, KeyW, ValueW *Tensor
	QueryB, KeyB, ValueB []float32

	AttnOutW *Tensor
	AttnOutB []float32

	Attn1LN_W, Attn1LN_B []float32 // attention output LayerNorm

	InterW *Tensor // intermediate.dense [H, FFN]
	InterB []float32

	OutputW *Tensor // output.dense [FFN, H]
	OutputB []float32

	Out2LN_W, Out2LN_B []float32 // output LayerNorm
}

// Model is a loaded distilroberta classifier.
type Model struct {
	Meta Meta

	WordEmb    *Tensor // [V, H]
	PosEmb     *Tensor // [MaxPos, H]
	TypeEmb    *Tensor // [1, H]
	EmbLN_W    []float32
	EmbLN_B    []float32

	Layers []Layer

	ClsDenseW  *Tensor // [H, H]
	ClsDenseB  []float32
	ClsOutW    *Tensor // [num_classes, H]  (transformers stores this as [H, num_classes] after transpose)
	ClsOutB    []float32
}

// LoadModel constructs a Model from an hbin Bundle. Every weight must be
// present with an expected name and shape; missing or mis-shaped weights
// return an error rather than silently zero-filling.
func LoadModel(b *Bundle) (*Model, error) {
	m := &Model{Meta: b.Meta}

	f32 := func(name string) ([]float32, error) {
		t, ok := b.Tensors[name]
		if !ok {
			return nil, fmt.Errorf("missing tensor: %s", name)
		}
		if t.DType != DTypeF32 {
			return nil, fmt.Errorf("%s: expected f32", name)
		}
		return t.F32, nil
	}
	ten := func(name string) (*Tensor, error) {
		t, ok := b.Tensors[name]
		if !ok {
			return nil, fmt.Errorf("missing tensor: %s", name)
		}
		if t.DType != DTypeF32 {
			return nil, fmt.Errorf("%s: expected f32", name)
		}
		return FromSlice(t.Shape, t.F32), nil
	}

	var err error
	if m.WordEmb, err = ten("m.roberta.embeddings.word_embeddings.weight"); err != nil {
		return nil, err
	}
	if m.PosEmb, err = ten("m.roberta.embeddings.position_embeddings.weight"); err != nil {
		return nil, err
	}
	if m.TypeEmb, err = ten("m.roberta.embeddings.token_type_embeddings.weight"); err != nil {
		return nil, err
	}
	if m.EmbLN_W, err = f32("m.roberta.embeddings.LayerNorm.weight"); err != nil {
		return nil, err
	}
	if m.EmbLN_B, err = f32("m.roberta.embeddings.LayerNorm.bias"); err != nil {
		return nil, err
	}

	m.Layers = make([]Layer, b.Meta.Layers)
	for i := 0; i < b.Meta.Layers; i++ {
		p := fmt.Sprintf("m.roberta.encoder.layer.%d.", i)
		L := &m.Layers[i]
		if L.QueryW, err = ten(p + "attention.self.query.weight"); err != nil {
			return nil, err
		}
		if L.KeyW, err = ten(p + "attention.self.key.weight"); err != nil {
			return nil, err
		}
		if L.ValueW, err = ten(p + "attention.self.value.weight"); err != nil {
			return nil, err
		}
		if L.QueryB, err = f32(p + "attention.self.query.bias"); err != nil {
			return nil, err
		}
		if L.KeyB, err = f32(p + "attention.self.key.bias"); err != nil {
			return nil, err
		}
		if L.ValueB, err = f32(p + "attention.self.value.bias"); err != nil {
			return nil, err
		}

		if L.AttnOutW, err = ten(p + "attention.output.dense.weight"); err != nil {
			return nil, err
		}
		if L.AttnOutB, err = f32(p + "attention.output.dense.bias"); err != nil {
			return nil, err
		}
		if L.Attn1LN_W, err = f32(p + "attention.output.LayerNorm.weight"); err != nil {
			return nil, err
		}
		if L.Attn1LN_B, err = f32(p + "attention.output.LayerNorm.bias"); err != nil {
			return nil, err
		}

		if L.InterW, err = ten(p + "intermediate.dense.weight"); err != nil {
			return nil, err
		}
		if L.InterB, err = f32(p + "intermediate.dense.bias"); err != nil {
			return nil, err
		}
		if L.OutputW, err = ten(p + "output.dense.weight"); err != nil {
			return nil, err
		}
		if L.OutputB, err = f32(p + "output.dense.bias"); err != nil {
			return nil, err
		}
		if L.Out2LN_W, err = f32(p + "output.LayerNorm.weight"); err != nil {
			return nil, err
		}
		if L.Out2LN_B, err = f32(p + "output.LayerNorm.bias"); err != nil {
			return nil, err
		}
	}

	if m.ClsDenseW, err = ten("m.classifier.dense.weight"); err != nil {
		return nil, err
	}
	if m.ClsDenseB, err = f32("m.classifier.dense.bias"); err != nil {
		return nil, err
	}
	if m.ClsOutW, err = ten("m.classifier.out_proj.weight"); err != nil {
		return nil, err
	}
	if m.ClsOutB, err = f32("m.classifier.out_proj.bias"); err != nil {
		return nil, err
	}

	return m, nil
}

// Forward runs a classifier forward pass for a single example.
// inputIDs and attentionMask are [seqLen]. Returns logits of length
// OutputClasses.
//
// The runtime trims trailing padding tokens before running. Transformers
// with masked attention are length invariant, so this does not change the
// numerics — only skips wasted computation on pad positions that would be
// zeroed out anyway. For typical hush inputs (~60 tokens out of 384) this
// is a 20-40x speedup.
func (m *Model) Forward(inputIDs, attentionMask []int32) []float32 {
	H := m.Meta.Hidden

	// Compute effective sequence length = 1 + index of last non-padding token.
	T := len(inputIDs)
	for T > 0 && attentionMask[T-1] == 0 {
		T--
	}
	if T == 0 {
		// Degenerate input; return zero logits rather than panicking.
		return make([]float32, m.Meta.OutputClasses)
	}
	if T < len(inputIDs) {
		inputIDs = inputIDs[:T]
		attentionMask = attentionMask[:T]
	}

	// --- embeddings ---
	// word_embeddings lookup + position_embeddings + token_type_embeddings
	x := Gather(m.WordEmb, inputIDs, 1, T) // [1, T, H]

	// RoBERTa position ids: for non-padding tokens, pos = padding_idx + cumulative_count.
	// Padding positions get padding_idx.
	posIDs := make([]int32, T)
	running := int32(0)
	pad := int32(m.Meta.PaddingIdx)
	for i := 0; i < T; i++ {
		if attentionMask[i] == 1 {
			running++
			posIDs[i] = pad + running
		} else {
			posIDs[i] = pad
		}
	}
	posEmbeds := Gather(m.PosEmb, posIDs, 1, T)
	AddInPlace(x, posEmbeds)

	// token_type_embeddings at index 0 (RoBERTa uses single type)
	typeIDs := make([]int32, T)
	typeEmbeds := Gather(m.TypeEmb, typeIDs, 1, T)
	AddInPlace(x, typeEmbeds)

	// embedding layer norm
	x = LayerNorm(x, m.EmbLN_W, m.EmbLN_B, 1e-5)

	// --- additive attention mask: 0 where keep, large negative where pad ---
	// shape [T] broadcast across heads and query positions
	addMask := make([]float32, T)
	for i := 0; i < T; i++ {
		if attentionMask[i] == 0 {
			addMask[i] = -1e9
		}
	}

	// --- transformer blocks ---
	for li := 0; li < m.Meta.Layers; li++ {
		x = m.forwardLayer(x, addMask, &m.Layers[li])
	}

	// --- classifier head: take CLS (index 0) ---
	cls := make([]float32, H)
	copy(cls, x.Data[:H])
	clsT := FromSlice([]int{1, H}, cls)

	// classifier head uses Gemm with transB=1 in the original ONNX: both
	// weights are stored in PyTorch layout [out, in] and need transposing
	// before matmul. pkg/classifier previously dodged this by handling
	// out_proj specially; we now do it consistently for both layers.
	denseW := Transpose(m.ClsDenseW, []int{1, 0}) // [H, H] -> [H, H] (still H,H but logically Wᵀ)
	h := MatMul(clsT, denseW)                     // [1, H]
	AddBias(h, m.ClsDenseB)
	for i := range h.Data {
		h.Data[i] = float32(math.Tanh(float64(h.Data[i])))
	}

	outW := Transpose(m.ClsOutW, []int{1, 0}) // [num_classes, H] -> [H, num_classes]
	out := MatMul(h, outW)                    // [1, num_classes]
	AddBias(out, m.ClsOutB)

	return out.Data
}

// forwardLayer runs a single transformer block.
func (m *Model) forwardLayer(x *Tensor, addMask []float32, L *Layer) *Tensor {
	H := m.Meta.Hidden
	heads := m.Meta.Heads
	d := H / heads
	T := x.Shape[1]
	scale := float32(1.0 / math.Sqrt(float64(d)))

	// Reshape x to 2D for matmuls.
	x2d := Reshape(x, T, H) // [T, H]

	// Q/K/V projections. Our exporter kept W as [H, H] matching ONNX layout
	// (MatMul(x, W)).
	q := MatMul(x2d, L.QueryW)
	AddBias(q, L.QueryB)
	k := MatMul(x2d, L.KeyW)
	AddBias(k, L.KeyB)
	v := MatMul(x2d, L.ValueW)
	AddBias(v, L.ValueB)

	// Reshape to [heads, T, d] by splitting last axis into [heads, d]
	// then transposing [T, heads, d] -> [heads, T, d].
	q3 := splitHeads(q, T, heads, d)
	k3 := splitHeads(k, T, heads, d)
	v3 := splitHeads(v, T, heads, d)

	// kT transpose per head: [heads, T, d] -> [heads, d, T]
	kT := Transpose(k3, []int{0, 2, 1})

	// scores = Q @ K^T / sqrt(d)
	scores := BatchMatMul(q3, kT) // [heads, T, T]
	ScaleInPlace(scores, scale)

	// Add mask over the last axis of scores (key positions):
	// mask is length T, pattern repeats across [heads, T, T].
	ApplyAdditiveMask(scores, addMask)

	attn := Softmax(scores)

	// attn @ V -> [heads, T, d]
	ctx := BatchMatMul(attn, v3)

	// combine heads: [heads, T, d] -> [T, heads, d] -> [T, H]
	ctxT := Transpose(ctx, []int{1, 0, 2})
	ctx2d := Reshape(ctxT, T, H)

	// attention output dense
	out := MatMul(ctx2d, L.AttnOutW)
	AddBias(out, L.AttnOutB)

	// residual + layernorm
	out = Add(out, x2d)
	out = LayerNorm(out, L.Attn1LN_W, L.Attn1LN_B, 1e-5)

	// FFN: intermediate (linear + GELU)
	inter := MatMul(out, L.InterW)
	AddBias(inter, L.InterB)
	inter = GELU(inter)

	// output dense
	ffnOut := MatMul(inter, L.OutputW)
	AddBias(ffnOut, L.OutputB)

	// residual + layernorm
	ffnOut = Add(ffnOut, out)
	ffnOut = LayerNorm(ffnOut, L.Out2LN_W, L.Out2LN_B, 1e-5)

	return Reshape(ffnOut, 1, T, H)
}

// splitHeads reshapes [T, H] -> [heads, T, d] where H = heads * d.
// It performs a transpose so heads become the leading dim.
func splitHeads(x *Tensor, T, heads, d int) *Tensor {
	// x view as [T, heads, d] -> transpose to [heads, T, d]
	return Transpose(Reshape(x, T, heads, d), []int{1, 0, 2})
}
