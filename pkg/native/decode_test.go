package native

import (
	"math"
	"testing"
)

// Shared label inventory for tests.
//   0 -> O
//   1 -> B-secret
//   2 -> I-secret
//   3 -> B-pii
//   4 -> I-pii
var testID2Label = map[int]string{
	0: "O",
	1: "B-secret",
	2: "I-secret",
	3: "B-pii",
	4: "I-pii",
}

const testK = 5

// buildLogits constructs a [T,K] logits slice where token i has argmax = ids[i].
// The winning logit gets `hi`, others get 0, so the softmax max prob is
// exp(hi) / (exp(hi) + (K-1)).
func buildLogits(ids []int, hi float32) []float32 {
	logits := make([]float32, len(ids)*testK)
	for i, id := range ids {
		for j := 0; j < testK; j++ {
			if j == id {
				logits[i*testK+j] = hi
			}
		}
	}
	return logits
}

func expectedProb(hi float32) float32 {
	e := math.Exp(float64(hi))
	return float32(e / (e + float64(testK-1)))
}

// makeOffsets creates simple non-zero offsets for "real" tokens. Pass
// (0,0) explicitly via the special slice if a token should be a special.
func makeOffsets(n int, specials map[int]bool) [][2]int {
	offs := make([][2]int, n)
	for i := 0; i < n; i++ {
		if specials[i] {
			offs[i] = [2]int{0, 0}
			continue
		}
		offs[i] = [2]int{i * 10, i*10 + 5}
	}
	return offs
}

func makeMask(n int, padded map[int]bool) []int32 {
	m := make([]int32, n)
	for i := 0; i < n; i++ {
		if padded[i] {
			m[i] = 0
		} else {
			m[i] = 1
		}
	}
	return m
}

func TestDecodeBIO_CleanBIO(t *testing.T) {
	// O, B-secret, I-secret, O, B-pii
	ids := []int{0, 1, 2, 0, 3}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := makeMask(len(ids), nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d: %+v", len(spans), spans)
	}
	if spans[0].Type != "secret" || spans[0].Start != 10 || spans[0].End != 25 {
		t.Errorf("span0 wrong: %+v", spans[0])
	}
	if spans[1].Type != "pii" || spans[1].Start != 40 || spans[1].End != 45 {
		t.Errorf("span1 wrong: %+v", spans[1])
	}
}

func TestDecodeBIO_LenientLeadingI(t *testing.T) {
	// O, I-secret, I-secret -> one secret span (lenient).
	ids := []int{0, 2, 2}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := makeMask(len(ids), nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d: %+v", len(spans), spans)
	}
	if spans[0].Type != "secret" || spans[0].Start != 10 || spans[0].End != 25 {
		t.Errorf("span wrong: %+v", spans[0])
	}
}

func TestDecodeBIO_TypeMismatch(t *testing.T) {
	// B-secret, I-pii -> two spans (secret then pii via lenient).
	ids := []int{1, 4}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := makeMask(len(ids), nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d: %+v", len(spans), spans)
	}
	if spans[0].Type != "secret" {
		t.Errorf("span0 type: %s", spans[0].Type)
	}
	if spans[1].Type != "pii" {
		t.Errorf("span1 type: %s", spans[1].Type)
	}
}

func TestDecodeBIO_SpecialTokenSplitsSpan(t *testing.T) {
	// B-secret, <special>, I-secret -> two spans (special closes, lenient reopens).
	ids := []int{1, 2, 2}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), map[int]bool{1: true})
	mask := makeMask(len(ids), nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d: %+v", len(spans), spans)
	}
	if spans[0].Type != "secret" || spans[0].Start != 0 || spans[0].End != 5 {
		t.Errorf("span0 wrong: %+v", spans[0])
	}
	if spans[1].Type != "secret" || spans[1].Start != 20 || spans[1].End != 25 {
		t.Errorf("span1 wrong: %+v", spans[1])
	}
}

func TestDecodeBIO_AllO(t *testing.T) {
	ids := []int{0, 0, 0, 0}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := makeMask(len(ids), nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 0 {
		t.Fatalf("want 0 spans, got %d", len(spans))
	}
}

func TestDecodeBIO_AllPad(t *testing.T) {
	ids := []int{1, 2, 3, 4}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := []int32{0, 0, 0, 0}

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 0 {
		t.Fatalf("want 0 spans, got %d", len(spans))
	}
}

func TestDecodeBIO_PaddingMidSequenceBoundary(t *testing.T) {
	// B-secret, I-secret(pad), I-secret -> pad closes, lenient reopens => 2 spans.
	ids := []int{1, 2, 2}
	logits := buildLogits(ids, 5.0)
	offs := makeOffsets(len(ids), nil)
	mask := []int32{1, 0, 1}

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d: %+v", len(spans), spans)
	}
	if spans[0].Type != "secret" || spans[0].Start != 0 || spans[0].End != 5 {
		t.Errorf("span0 wrong: %+v", spans[0])
	}
	if spans[1].Type != "secret" || spans[1].Start != 20 || spans[1].End != 25 {
		t.Errorf("span1 wrong: %+v", spans[1])
	}
}

func TestDecodeBIO_ScoreIsMean(t *testing.T) {
	// Two B-secret tokens with different "hi" -> different probs; check mean.
	// Token0: argmax 1 with hi=5 ; Token1: argmax 2 with hi=2.
	logits := make([]float32, 2*testK)
	logits[0*testK+1] = 5.0
	logits[1*testK+2] = 2.0
	offs := makeOffsets(2, nil)
	mask := makeMask(2, nil)

	spans := DecodeBIO(logits, testK, testID2Label, offs, mask)
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d: %+v", len(spans), spans)
	}
	want := (expectedProb(5.0) + expectedProb(2.0)) / 2
	got := spans[0].Score
	diff := float64(got - want)
	if diff < 0 {
		diff = -diff
	}
	if diff > 1e-5 {
		t.Errorf("score mean: want %v, got %v", want, got)
	}
}
