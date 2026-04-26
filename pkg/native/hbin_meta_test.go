package native

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMetaIsTokenClassificationV1Default(t *testing.T) {
	// v1 hbin files have no Task field — should default to false.
	m := &Meta{
		Model:         "distilroberta",
		Hidden:        768,
		OutputClasses: 2,
	}
	if m.IsTokenClassification() {
		t.Fatalf("v1 meta with empty Task should not be token classification")
	}
}

func TestMetaIsTokenClassificationV2NER(t *testing.T) {
	m := &Meta{
		Task: "token_classification",
		Labels: &Labels{
			NumLabels: 9,
			SeqLen:    256,
		},
	}
	if !m.IsTokenClassification() {
		t.Fatalf("Task=token_classification should report true")
	}
}

func TestMetaJSONRoundTripWithLabels(t *testing.T) {
	orig := Meta{
		Model:         "distilroberta",
		Hidden:        768,
		Layers:        6,
		Heads:         12,
		FFN:           3072,
		Vocab:         50265,
		MaxPosition:   514,
		PaddingIdx:    1,
		SeqLen:        256,
		OutputClasses: 9,
		Task:          "token_classification",
		Labels: &Labels{
			Id2Label: map[string]string{
				"0": "O",
				"1": "B-SECRET",
				"2": "I-SECRET",
			},
			Label2Id: map[string]int{
				"O":        0,
				"B-SECRET": 1,
				"I-SECRET": 2,
			},
			NumLabels: 3,
			SeqLen:    256,
		},
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Meta
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Fatalf("round trip mismatch:\n want %#v\n  got %#v", orig, got)
	}
	if !got.IsTokenClassification() {
		t.Fatalf("round-tripped meta should be token classification")
	}
	if got.Labels == nil || got.Labels.NumLabels != 3 {
		t.Fatalf("labels not preserved: %+v", got.Labels)
	}
}

func TestMetaJSONOmitsEmptyTaskAndLabels(t *testing.T) {
	// Sequence classification meta — no Task, no Labels — must serialize
	// without those fields so we don't bloat existing v1 hbin headers.
	m := Meta{Model: "distilroberta", OutputClasses: 2}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if contains(s, `"task"`) {
		t.Fatalf("expected omitempty task, got %s", s)
	}
	if contains(s, `"labels"`) {
		t.Fatalf("expected omitempty labels, got %s", s)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
