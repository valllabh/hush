package scanner

import (
	"fmt"
	"io"
	"strings"
)

// Options configures a Scanner.
//
// Zero values are sensible defaults — these match the CLI:
//
//	MinConfidence = 0.5    keep findings scoring >= 0.5
//	EntropyThreshold = 4.0 shannon entropy floor for generic candidates
//	CtxChars = 256         chars of left/right context passed to the scorer
//	ModelOff = false       classifier enabled; set true to skip ML filtering
//	IntraOpThreads = 0     ORT default (one thread per CPU)
type Options struct {
	MinConfidence    float64
	EntropyThreshold float64
	CtxChars         int
	ModelOff         bool
	IntraOpThreads   int
}

func (o Options) withDefaults() Options {
	if o.MinConfidence == 0 {
		o.MinConfidence = 0.5
	}
	if o.EntropyThreshold == 0 {
		o.EntropyThreshold = 4.0
	}
	if o.CtxChars == 0 {
		o.CtxChars = 256
	}
	return o
}

// Scanner is a stateful, ergonomic facade over Scan + MaskText.
//
// It owns the classifier lifecycle so callers do not have to wire one up
// by hand. Safe for concurrent use.
type Scanner struct {
	opts   Options
	scorer Scorer
	closer func() error
}

// New returns a ready to use Scanner. When ModelOff is false it loads the
// embedded classifier via the default factory (see DefaultScorerFactory).
// Callers must Close() the scanner to release the ONNX session.
func New(opts Options) (*Scanner, error) {
	opts = opts.withDefaults()
	s := &Scanner{opts: opts}
	if !opts.ModelOff {
		if DefaultScorerFactory == nil {
			return nil, fmt.Errorf("no scorer factory registered; build with classifier or set ModelOff=true")
		}
		scorer, closer, err := DefaultScorerFactory(opts.IntraOpThreads)
		if err != nil {
			return nil, err
		}
		s.scorer = scorer
		s.closer = closer
	}
	return s, nil
}

// Close releases the classifier. Safe to call multiple times.
func (s *Scanner) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	err := s.closer()
	s.closer = nil
	return err
}

// ScanString scans a string, returning findings.
func (s *Scanner) ScanString(text string) ([]Finding, error) {
	return Scan(text, s.opts.MinConfidence, s.opts.EntropyThreshold, s.opts.CtxChars, s.scorer)
}

// ScanReader reads the whole stream then scans. Suitable for small-to-medium
// inputs (typical files). For true streaming use ScanString in chunks.
func (s *Scanner) ScanReader(r io.Reader) ([]Finding, error) {
	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		return nil, err
	}
	return s.ScanString(b.String())
}

// BatchScore scores many candidate spans in a single transformer forward
// pass when the underlying scorer supports it. Falls back to per-candidate
// Score when it doesn't. Returns probabilities in input order.
//
// Useful when a caller already has a list of (left, span, right) triples
// from their own extraction pipeline and wants to skip hush's regex stage.
func (s *Scanner) BatchScore(triples []SpanTriple) ([]float64, error) {
	if s.scorer == nil {
		out := make([]float64, len(triples))
		for i := range out {
			out[i] = 1.0
		}
		return out, nil
	}
	if bs, ok := s.scorer.(BatchScorer); ok {
		return bs.BatchScore(triples)
	}
	out := make([]float64, len(triples))
	for i, t := range triples {
		p, err := s.scorer.Score(t.Left, t.Span, t.Right)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

// Redact scans text and returns a masked copy with each finding replaced.
// placeholder supports the %s verb, which is substituted with the rule name.
// An empty placeholder falls back to the default [REDACTED_<RULE>_<N>].
func (s *Scanner) Redact(text string, placeholder string) (string, []Finding, error) {
	findings, err := s.ScanString(text)
	if err != nil {
		return "", nil, err
	}
	masked := maskTextFormatted(text, findings, placeholder)
	return masked, findings, nil
}

// maskTextFormatted is like MaskText but honours a printf style placeholder.
func maskTextFormatted(text string, findings []Finding, placeholder string) string {
	if len(findings) == 0 {
		return text
	}
	if !strings.Contains(placeholder, "%s") {
		return MaskText(text, findings, placeholder)
	}
	// Per finding rendering: substitute the rule name into placeholder.
	fixed := make([]Finding, len(findings))
	copy(fixed, findings)
	// Reuse MaskText by passing empty placeholder and post-rewriting is hard;
	// simpler: replicate MaskText logic locally with per finding placeholder.
	var b strings.Builder
	b.Grow(len(text))
	// sort findings by Start (MaskText does this too; re import is circular)
	for i := 0; i < len(fixed); i++ {
		for j := i + 1; j < len(fixed); j++ {
			if fixed[j].Start < fixed[i].Start {
				fixed[i], fixed[j] = fixed[j], fixed[i]
			}
		}
	}
	prev := 0
	for _, f := range fixed {
		if f.Start < prev {
			continue
		}
		b.WriteString(text[prev:f.Start])
		b.WriteString(fmt.Sprintf(placeholder, f.Rule))
		prev = f.End
	}
	b.WriteString(text[prev:])
	return b.String()
}

// ScorerFactory returns a Scorer plus a closer callback. The closer is
// invoked by Scanner.Close(). Set DefaultScorerFactory from a package that
// depends on the classifier (e.g. pkg/classifier's init or a small shim
// in cmd/hush) so pkg/scanner stays decoupled from ORT.
type ScorerFactory func(intraOpThreads int) (Scorer, func() error, error)

// DefaultScorerFactory is nil by default. Applications that want the
// embedded classifier should import a package that sets this.
var DefaultScorerFactory ScorerFactory
