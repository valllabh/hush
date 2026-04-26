package native

import (
	"math"
	"strings"
)

// Span represents a decoded entity span over character offsets.
type Span struct {
	Start int     // char offset, inclusive
	End   int     // char offset, exclusive
	Type  string  // entity type from BIO tag, e.g. "secret", "pii", "noise"
	Score float32 // mean softmax-max prob across span tokens
}

// DecodeBIO greedy-argmaxes per-token logits, drops O, merges contiguous
// B-X / I-X runs into a single Span. Lenient: I-X with no preceding B-X
// (or following a different type) is treated as B-X.
//
// logits is row-major [T, K]. offsets[i] == [2]int{0,0} marks special
// tokens (skip). attentionMask[i] == 0 marks padding (skip).
func DecodeBIO(
	logits []float32,
	K int,
	id2label map[int]string,
	offsets [][2]int,
	attentionMask []int32,
) []Span {
	if K <= 0 || len(logits) == 0 {
		return nil
	}
	T := len(logits) / K
	if T == 0 {
		return nil
	}

	var spans []Span
	var cur *Span
	var probs []float32

	closeSpan := func() {
		if cur == nil {
			return
		}
		if len(probs) > 0 {
			var sum float32
			for _, p := range probs {
				sum += p
			}
			cur.Score = sum / float32(len(probs))
		}
		spans = append(spans, *cur)
		cur = nil
		probs = nil
	}

	for i := 0; i < T; i++ {
		// Skip special tokens / padding (close any open span).
		if i < len(attentionMask) && attentionMask[i] == 0 {
			closeSpan()
			continue
		}
		if i < len(offsets) && offsets[i][0] == 0 && offsets[i][1] == 0 {
			closeSpan()
			continue
		}

		row := logits[i*K : (i+1)*K]

		// Numerically stable softmax: find max, then argmax + max prob.
		maxLogit := row[0]
		argmax := 0
		for j := 1; j < K; j++ {
			if row[j] > maxLogit {
				maxLogit = row[j]
				argmax = j
			}
		}
		var denom float64
		for j := 0; j < K; j++ {
			denom += math.Exp(float64(row[j] - maxLogit))
		}
		prob := float32(1.0 / denom) // exp(maxLogit - maxLogit) / denom

		label, ok := id2label[argmax]
		if !ok || label == "" || label == "O" {
			closeSpan()
			continue
		}

		var prefix, etype string
		if idx := strings.IndexByte(label, '-'); idx >= 0 {
			prefix = label[:idx]
			etype = label[idx+1:]
		} else {
			// Unknown label format; treat as O.
			closeSpan()
			continue
		}

		start := 0
		end := 0
		if i < len(offsets) {
			start = offsets[i][0]
			end = offsets[i][1]
		}

		switch prefix {
		case "B":
			closeSpan()
			cur = &Span{Start: start, End: end, Type: etype}
			probs = []float32{prob}
		case "I":
			if cur != nil && cur.Type == etype {
				cur.End = end
				probs = append(probs, prob)
			} else {
				// Lenient: treat as B-X.
				closeSpan()
				cur = &Span{Start: start, End: end, Type: etype}
				probs = []float32{prob}
			}
		default:
			closeSpan()
		}
	}
	closeSpan()
	return spans
}
