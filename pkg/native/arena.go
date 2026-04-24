package native

import "sync"

var arenaPool = sync.Pool{
	New: func() any { return &Arena{} },
}

// getArena pulls a reset arena from the pool.
func getArena() *Arena {
	a := arenaPool.Get().(*Arena)
	a.Reset()
	return a
}

// putArena returns an arena to the pool for reuse on the next forward.
func putArena(a *Arena) {
	arenaPool.Put(a)
}

// Arena is a per-forward scratch allocator for *Tensor buffers. The
// forward pass creates hundreds of short-lived tensors (one per op);
// allocating each via make/NewTensor turned into ~500 allocs and ~4 MB
// per call, all of which then had to be GC'd.
//
// The arena hands out []float32 buffers from a pool of reusable slices,
// growing the pool only when a request exceeds all existing buffers.
// Reset() marks every buffer free so the next forward reuses them in
// place. Allocation is O(B) per Get where B is the number of live
// buffers; B stays tiny (tens).
//
// This is explicitly not thread-safe. Each forward pass owns its
// arena; parallel inference creates multiple arenas.
type Arena struct {
	// bufs is the pool. Each entry is a float32 slice whose len is the
	// capacity the caller requested at Get time and whose cap is the
	// underlying allocated size. Once returned by Get, the buffer is
	// considered "in use" until Reset.
	bufs [][]float32
	// inUse[i] is true while bufs[i] is handed out.
	inUse []bool
	// tensorChunks pools Tensor structs in fixed-size slabs so pointers
	// returned from Get remain stable across subsequent Get calls.
	tensorChunks [][]Tensor
	tensorNext   int // index within the most recent chunk
	// shapeChunks backs Shape slices; each is a fixed-size []int slab
	// for the same stability reason.
	shapeChunks [][]int
	shapeNext   int
}

const arenaTensorChunk = 256
const arenaShapeChunk = 1024

// NewArena creates an empty arena.
func NewArena() *Arena { return &Arena{} }

// getBuf returns a []float32 of length n with contents zeroed, reusing
// an existing backing slice when possible.
func (a *Arena) getBuf(n int) []float32 {
	// First fit: smallest free buffer whose cap >= n.
	bestIdx := -1
	for i, b := range a.bufs {
		if a.inUse[i] {
			continue
		}
		if cap(b) < n {
			continue
		}
		if bestIdx == -1 || cap(b) < cap(a.bufs[bestIdx]) {
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		a.inUse[bestIdx] = true
		b := a.bufs[bestIdx][:n]
		// zero the slice (matches NewTensor semantics)
		for i := range b {
			b[i] = 0
		}
		a.bufs[bestIdx] = b
		return b
	}
	// Grow: allocate a new buffer.
	b := make([]float32, n)
	a.bufs = append(a.bufs, b)
	a.inUse = append(a.inUse, true)
	return b
}

// Get returns a zeroed *Tensor of the given shape backed by pool memory.
// The tensor is valid until the next Reset() call on this arena.
func (a *Arena) Get(shape ...int) *Tensor {
	n := 1
	for _, d := range shape {
		n *= d
	}
	// Pull a Tensor from the chunked pool.
	if len(a.tensorChunks) == 0 || a.tensorNext >= len(a.tensorChunks[len(a.tensorChunks)-1]) {
		a.tensorChunks = append(a.tensorChunks, make([]Tensor, arenaTensorChunk))
		a.tensorNext = 0
	}
	chunk := a.tensorChunks[len(a.tensorChunks)-1]
	t := &chunk[a.tensorNext]
	a.tensorNext++
	// Shape slice from the shape slab pool.
	if len(a.shapeChunks) == 0 || a.shapeNext+len(shape) > len(a.shapeChunks[len(a.shapeChunks)-1]) {
		sz := arenaShapeChunk
		if len(shape) > sz {
			sz = len(shape)
		}
		a.shapeChunks = append(a.shapeChunks, make([]int, sz))
		a.shapeNext = 0
	}
	sc := a.shapeChunks[len(a.shapeChunks)-1]
	copy(sc[a.shapeNext:], shape)
	t.Shape = sc[a.shapeNext : a.shapeNext+len(shape) : a.shapeNext+len(shape)]
	a.shapeNext += len(shape)
	t.Data = a.getBuf(n)
	t.Packed = nil
	return t
}

// view returns a *Tensor with its Shape set but no Data buffer. The
// caller is responsible for setting Data (typically a shared view of
// another tensor's slice). Used by Reshape/view-style ops that don't
// allocate their own storage.
func (a *Arena) view(shape []int) *Tensor {
	if len(a.tensorChunks) == 0 || a.tensorNext >= len(a.tensorChunks[len(a.tensorChunks)-1]) {
		a.tensorChunks = append(a.tensorChunks, make([]Tensor, arenaTensorChunk))
		a.tensorNext = 0
	}
	chunk := a.tensorChunks[len(a.tensorChunks)-1]
	t := &chunk[a.tensorNext]
	a.tensorNext++
	if len(a.shapeChunks) == 0 || a.shapeNext+len(shape) > len(a.shapeChunks[len(a.shapeChunks)-1]) {
		sz := arenaShapeChunk
		if len(shape) > sz {
			sz = len(shape)
		}
		a.shapeChunks = append(a.shapeChunks, make([]int, sz))
		a.shapeNext = 0
	}
	sc := a.shapeChunks[len(a.shapeChunks)-1]
	copy(sc[a.shapeNext:], shape)
	t.Shape = sc[a.shapeNext : a.shapeNext+len(shape) : a.shapeNext+len(shape)]
	a.shapeNext += len(shape)
	t.Data = nil
	t.Packed = nil
	return t
}

// Reset marks all buffers free. Data remains in the underlying slices;
// Get will zero it on re-issue. Call at the end of each forward pass.
func (a *Arena) Reset() {
	for i := range a.inUse {
		a.inUse[i] = false
	}
	// Collapse the chunked pools back to the first chunk; keep allocated
	// memory for reuse on the next forward pass.
	if len(a.tensorChunks) > 1 {
		a.tensorChunks = a.tensorChunks[:1]
	}
	a.tensorNext = 0
	if len(a.shapeChunks) > 1 {
		a.shapeChunks = a.shapeChunks[:1]
	}
	a.shapeNext = 0
}
