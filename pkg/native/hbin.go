// Package native is a pure Go transformer inference runtime for hush.
//
// It loads a distilroberta model from the .hbin format produced by
// training/scripts/export_hbin.py and runs a classifier forward pass
// without any CGO dependency. This exists so hush can ship as a single
// static binary with no libonnxruntime requirement at runtime.
package native

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"unsafe"
)

const (
	DTypeF32 uint8 = 1
	DTypeI8  uint8 = 2
	DTypeI32 uint8 = 3
)

// Meta captures the topology of the embedded model. The hbin exporter
// infers these from the ONNX graph and writes them as JSON in the header.
type Meta struct {
	Model          string `json:"model"`
	Hidden         int    `json:"hidden"`
	Layers         int    `json:"layers"`
	Heads          int    `json:"heads"`
	FFN            int    `json:"ffn"`
	Vocab          int    `json:"vocab"`
	MaxPosition    int    `json:"max_position"`
	TokenTypeCount int    `json:"token_type_count"`
	PaddingIdx     int    `json:"padding_idx"`
	SeqLen         int    `json:"seq_len"`
	OutputClasses  int    `json:"output_classes"`
}

// RawTensor is a weight tensor read straight from the hbin file.
type RawTensor struct {
	Name  string
	DType uint8
	Shape []int
	F32   []float32 // populated when DType == DTypeF32
	I8    []int8    // populated when DType == DTypeI8
	I32   []int32   // populated when DType == DTypeI32
}

// Bundle is the result of reading an hbin file: topology + all tensors
// keyed by their ONNX initializer names.
type Bundle struct {
	Meta    Meta
	Tensors map[string]*RawTensor
}

// Read parses an hbin stream.
func Read(r io.Reader) (*Bundle, error) {
	br := &byteReader{r: r}
	magic := br.bytes(4)
	if br.err != nil || string(magic) != "HBIN" {
		return nil, fmt.Errorf("not an hbin file: magic=%q", magic)
	}
	version := br.u32()
	if version != 1 {
		return nil, fmt.Errorf("unsupported hbin version %d", version)
	}
	tensorCount := br.u32()
	metaLen := br.u32()
	metaBytes := br.bytes(int(metaLen))
	if br.err != nil {
		return nil, br.err
	}
	var meta Meta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("bad meta json: %w", err)
	}
	b := &Bundle{Meta: meta, Tensors: make(map[string]*RawTensor, tensorCount)}
	for i := uint32(0); i < tensorCount; i++ {
		t, err := readTensor(br)
		if err != nil {
			return nil, fmt.Errorf("tensor %d: %w", i, err)
		}
		b.Tensors[t.Name] = t
	}
	return b, nil
}

func readTensor(br *byteReader) (*RawTensor, error) {
	nameLen := br.u32()
	name := string(br.bytes(int(nameLen)))
	dtype := br.u8()
	rank := br.u8()
	shape := make([]int, rank)
	for i := range shape {
		shape[i] = int(br.u32())
	}
	dataLen := br.u64()
	raw := br.bytes(int(dataLen))
	if br.err != nil {
		return nil, br.err
	}
	t := &RawTensor{Name: name, DType: dtype, Shape: shape}
	switch dtype {
	case DTypeF32:
		// Reinterpret bytes as []float32 without copying.
		if len(raw)%4 != 0 {
			return nil, fmt.Errorf("f32 payload not a multiple of 4")
		}
		t.F32 = bytesToF32(raw)
	case DTypeI8:
		t.I8 = bytesToI8(raw)
	case DTypeI32:
		if len(raw)%4 != 0 {
			return nil, fmt.Errorf("i32 payload not a multiple of 4")
		}
		t.I32 = bytesToI32(raw)
	default:
		return nil, fmt.Errorf("unknown dtype %d", dtype)
	}
	expected := 1
	for _, d := range shape {
		expected *= d
	}
	if n := t.numel(); n != expected {
		return nil, fmt.Errorf("%s: shape %v needs %d scalars, got %d", name, shape, expected, n)
	}
	return t, nil
}

func (t *RawTensor) numel() int {
	switch t.DType {
	case DTypeF32:
		return len(t.F32)
	case DTypeI8:
		return len(t.I8)
	case DTypeI32:
		return len(t.I32)
	}
	return 0
}

// -- byte reader helpers ----

type byteReader struct {
	r   io.Reader
	err error
	buf [8]byte
}

func (b *byteReader) bytes(n int) []byte {
	if b.err != nil {
		return nil
	}
	out := make([]byte, n)
	if _, err := io.ReadFull(b.r, out); err != nil {
		b.err = err
		return nil
	}
	return out
}

func (b *byteReader) u8() uint8 {
	if b.err != nil {
		return 0
	}
	if _, err := io.ReadFull(b.r, b.buf[:1]); err != nil {
		b.err = err
		return 0
	}
	return b.buf[0]
}

func (b *byteReader) u32() uint32 {
	if b.err != nil {
		return 0
	}
	if _, err := io.ReadFull(b.r, b.buf[:4]); err != nil {
		b.err = err
		return 0
	}
	return binary.LittleEndian.Uint32(b.buf[:4])
}

func (b *byteReader) u64() uint64 {
	if b.err != nil {
		return 0
	}
	if _, err := io.ReadFull(b.r, b.buf[:8]); err != nil {
		b.err = err
		return 0
	}
	return binary.LittleEndian.Uint64(b.buf[:8])
}

// -- unsafe byte reinterpretation (LE only, caller owns lifetime) ----

func bytesToF32(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	return (*[1 << 30]float32)(unsafe.Pointer(&b[0]))[:n:n]
}

func bytesToI8(b []byte) []int8 {
	if len(b) == 0 {
		return nil
	}
	return (*[1 << 30]int8)(unsafe.Pointer(&b[0]))[:len(b):len(b)]
}

func bytesToI32(b []byte) []int32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	return (*[1 << 30]int32)(unsafe.Pointer(&b[0]))[:n:n]
}
