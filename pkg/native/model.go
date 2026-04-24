package native

import (
	"fmt"
	"math"
)

// Layer holds all weights for one transformer block.
type Layer struct {
	QueryW, KeyW, ValueW *MaybeWeight
	QueryB, KeyB, ValueB []float32

	AttnOutW *MaybeWeight
	AttnOutB []float32

	Attn1LN_W, Attn1LN_B []float32 // attention output LayerNorm

	InterW *MaybeWeight // intermediate.dense [H, FFN]
	InterB []float32

	OutputW *MaybeWeight // output.dense [FFN, H]
	OutputB []float32

	Out2LN_W, Out2LN_B []float32 // output LayerNorm
}

// Model is a loaded distilroberta classifier.
type Model struct {
	Meta Meta

	WordEmb *Tensor // [V, H]
	PosEmb  *Tensor // [MaxPos, H]
	TypeEmb *Tensor // [1, H]
	EmbLN_W []float32
	EmbLN_B []float32

	Layers []Layer

	// Classifier dense + out_proj. Stored so that MatMul(a, W) produces
	// the expected output: for fp32 this is the already-transposed [in, out]
	// layout; for int8 the exporter pre-transposes so the stored weight is
	// also [in, out].
	ClsDenseW *MaybeWeight // [H, H]
	ClsDenseB []float32
	ClsOutW   *MaybeWeight // [H, num_classes]
	ClsOutB   []float32
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
	// embTen loads an embedding table that may be fp32 or per-row int8.
	// For int8: shape [R, C], companion "<name>.scale" fp32 [R], dequant is
	//   W[r, c] = q[r, c] * scale[r]
	// Eagerly materializes to fp32 so Gather stays unchanged.
	embTen := func(name string) (*Tensor, error) {
		t, ok := b.Tensors[name]
		if !ok {
			return nil, fmt.Errorf("missing tensor: %s", name)
		}
		switch t.DType {
		case DTypeF32:
			return FromSlice(t.Shape, t.F32), nil
		case DTypeI8:
			s, ok := b.Tensors[name+".scale"]
			if !ok {
				return nil, fmt.Errorf("int8 embedding %s missing scale", name)
			}
			if s.DType != DTypeF32 {
				return nil, fmt.Errorf("%s.scale: expected f32", name)
			}
			if len(t.Shape) != 2 {
				return nil, fmt.Errorf("%s: expected 2D int8 embedding, got %v", name, t.Shape)
			}
			R, C := t.Shape[0], t.Shape[1]
			if len(s.F32) != R {
				return nil, fmt.Errorf("%s.scale: len %d != rows %d", name, len(s.F32), R)
			}
			out := NewTensor(R, C)
			for r := 0; r < R; r++ {
				sc := s.F32[r]
				src := t.I8[r*C : (r+1)*C]
				dst := out.Data[r*C : (r+1)*C]
				for c := 0; c < C; c++ {
					dst[c] = float32(src[c]) * sc
				}
			}
			return out, nil
		default:
			return nil, fmt.Errorf("%s: unsupported dtype %d for embedding", name, t.DType)
		}
	}
	// mmw loads a matmul weight that may be fp32 or int8 quantized.
	// The stored layout is always [In, Out] matching MatMul(x, W).
	//
	// Int8 weights are EAGERLY dequantized to fp32 at load time so the
	// forward path runs the tuned fp32 matmul without per-call dequant
	// allocation. This trades 4x in RAM for matching fp32 throughput;
	// on the current model the matmul weights are ~75 MB int8 or ~300 MB
	// fp32, which is still fine for a CLI. Callers who want the int8
	// kernel live can call NewQuantWeight directly.
	mmw := func(name string) (*MaybeWeight, error) {
		t, ok := b.Tensors[name]
		if !ok {
			return nil, fmt.Errorf("missing tensor: %s", name)
		}
		switch t.DType {
		case DTypeF32:
			tt := FromSlice(t.Shape, t.F32)
			tt.PackForMatMul()
			return &MaybeWeight{F32: tt}, nil
		case DTypeI8:
			s, ok := b.Tensors[name+".scale"]
			if !ok {
				return nil, fmt.Errorf("int8 weight %s missing scale", name)
			}
			if s.DType != DTypeF32 {
				return nil, fmt.Errorf("%s.scale: expected f32", name)
			}
			if len(t.Shape) != 2 {
				return nil, fmt.Errorf("%s: expected 2D int8, got %v", name, t.Shape)
			}
			in, out := t.Shape[0], t.Shape[1]
			qw, err := NewQuantWeight(in, out, t.I8, s.F32)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			// Eager dequant: one pass at load, zero cost at forward.
			fp := qw.DequantizeToF32()
			fp.PackForMatMul()
			return &MaybeWeight{F32: fp}, nil
		default:
			return nil, fmt.Errorf("%s: unsupported dtype %d for matmul weight", name, t.DType)
		}
	}
	// mmwTransposed loads a matmul weight that, if stored as fp32, is in
	// [out, in] layout (PyTorch convention) and needs a transpose before
	// use. If stored as int8, the exporter already transposed it to
	// [in, out], so we return as-is.
	mmwTransposed := func(name string) (*MaybeWeight, error) {
		t, ok := b.Tensors[name]
		if !ok {
			return nil, fmt.Errorf("missing tensor: %s", name)
		}
		switch t.DType {
		case DTypeF32:
			src := FromSlice(t.Shape, t.F32)
			tt := Transpose(src, []int{1, 0})
			tt.PackForMatMul()
			return &MaybeWeight{F32: tt}, nil
		case DTypeI8:
			return mmw(name)
		default:
			return nil, fmt.Errorf("%s: unsupported dtype %d", name, t.DType)
		}
	}

	var err error
	if m.WordEmb, err = embTen("m.roberta.embeddings.word_embeddings.weight"); err != nil {
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
		if L.QueryW, err = mmw(p + "attention.self.query.weight"); err != nil {
			return nil, err
		}
		if L.KeyW, err = mmw(p + "attention.self.key.weight"); err != nil {
			return nil, err
		}
		if L.ValueW, err = mmw(p + "attention.self.value.weight"); err != nil {
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

		if L.AttnOutW, err = mmw(p + "attention.output.dense.weight"); err != nil {
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

		if L.InterW, err = mmw(p + "intermediate.dense.weight"); err != nil {
			return nil, err
		}
		if L.InterB, err = f32(p + "intermediate.dense.bias"); err != nil {
			return nil, err
		}
		if L.OutputW, err = mmw(p + "output.dense.weight"); err != nil {
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

	// Classifier: stored as [out, in] when fp32 (original PyTorch layout);
	// the int8 exporter transposes to [in, out] before quantization.
	if m.ClsDenseW, err = mmwTransposed("m.classifier.dense.weight"); err != nil {
		return nil, err
	}
	if m.ClsDenseB, err = f32("m.classifier.dense.bias"); err != nil {
		return nil, err
	}
	if m.ClsOutW, err = mmwTransposed("m.classifier.out_proj.weight"); err != nil {
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
	ar := getArena()
	defer putArena(ar)

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
	x := gatherArena(ar, m.WordEmb, inputIDs, 1, T) // [1, T, H]

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
	posEmbeds := gatherArena(ar, m.PosEmb, posIDs, 1, T)
	AddInPlace(x, posEmbeds)

	// token_type_embeddings at index 0 (RoBERTa uses single type)
	typeIDs := make([]int32, T)
	typeEmbeds := gatherArena(ar, m.TypeEmb, typeIDs, 1, T)
	AddInPlace(x, typeEmbeds)

	// embedding layer norm
	x = layerNormArena(ar, x, m.EmbLN_W, m.EmbLN_B, 1e-5)

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
		x = m.forwardLayer(ar, x, addMask, &m.Layers[li])
	}

	// --- classifier head: take CLS (index 0) ---
	cls := make([]float32, H)
	copy(cls, x.Data[:H])
	clsT := FromSlice([]int{1, H}, cls)

	// Dense [H, H] then tanh; both fp32 and int8 paths expose [in, out]
	// layout via MaybeWeight.MatMul.
	h := m.ClsDenseW.MatMul(clsT) // [1, H]
	AddBias(h, m.ClsDenseB)
	for i := range h.Data {
		h.Data[i] = float32(math.Tanh(float64(h.Data[i])))
	}

	out := m.ClsOutW.MatMul(h) // [1, num_classes]
	AddBias(out, m.ClsOutB)

	return out.Data
}

// forwardLayer runs a single transformer block.
func (m *Model) forwardLayer(ar *Arena, x *Tensor, addMask []float32, L *Layer) *Tensor {
	H := m.Meta.Hidden
	heads := m.Meta.Heads
	d := H / heads
	T := x.Shape[1]
	scale := float32(1.0 / math.Sqrt(float64(d)))

	// Reshape x to 2D for matmuls.
	x2d := reshapeArena(ar, x, T, H) // [T, H]

	// Q/K/V projections.
	q := L.QueryW.matMulArena(ar, x2d)
	AddBias(q, L.QueryB)
	k := L.KeyW.matMulArena(ar, x2d)
	AddBias(k, L.KeyB)
	v := L.ValueW.matMulArena(ar, x2d)
	AddBias(v, L.ValueB)

	q3 := splitHeads(ar, q, T, heads, d)
	k3 := splitHeads(ar, k, T, heads, d)
	v3 := splitHeads(ar, v, T, heads, d)

	kT := transposeArena(ar, k3, []int{0, 2, 1})

	scores := batchMatMulArena(ar, q3, kT)
	ScaleInPlace(scores, scale)
	ApplyAdditiveMask(scores, addMask)

	attn := softmaxArena(ar, scores)

	ctx := batchMatMulArena(ar, attn, v3)

	ctxT := transposeArena(ar, ctx, []int{1, 0, 2})
	ctx2d := reshapeArena(ar, ctxT, T, H)

	out := L.AttnOutW.matMulArena(ar, ctx2d)
	AddBias(out, L.AttnOutB)

	out = addArena(ar, out, x2d)
	out = layerNormArena(ar, out, L.Attn1LN_W, L.Attn1LN_B, 1e-5)

	inter := L.InterW.matMulArena(ar, out)
	AddBias(inter, L.InterB)
	inter = geluArena(ar, inter)

	ffnOut := L.OutputW.matMulArena(ar, inter)
	AddBias(ffnOut, L.OutputB)

	ffnOut = addArena(ar, ffnOut, out)
	ffnOut = layerNormArena(ar, ffnOut, L.Out2LN_W, L.Out2LN_B, 1e-5)

	return reshapeArena(ar, ffnOut, 1, T, H)
}

// splitHeads reshapes [T, H] -> [heads, T, d] where H = heads * d.
func splitHeads(ar *Arena, x *Tensor, T, heads, d int) *Tensor {
	return transposeArena(ar, reshapeArena(ar, x, T, heads, d), []int{1, 0, 2})
}
