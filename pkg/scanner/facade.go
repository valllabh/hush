package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/valllabh/hush/pkg/extractor"
)

// DetectedSpan is the minimal shape a Detector returns. Mirrors
// native.Span without importing pkg/native (that would form a cycle:
// pkg/native already imports pkg/scanner).
type DetectedSpan struct {
	Start int
	End   int
	Type  string
	Score float32
}

// Detector is the v2 NER pipeline contract. Any type that implements
// Detect(text) -> []DetectedSpan satisfies it; pkg/native.Detector does
// (via a thin adapter wrapped by callers). Kept minimal so pkg/scanner
// stays independent of any concrete model runtime.
type Detector interface {
	Detect(text string) ([]DetectedSpan, error)
}

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

	// DetectorPrefilter, when true, runs the regex+entropy extractor first
	// in v2 NER mode and only invokes the detector on text regions
	// containing candidates. Cuts cost on clean files from ~2.6s/4KB to
	// near zero by skipping the model entirely when no regex/entropy hit
	// fires. The detector still has final say over what the spans are.
	// Has no effect when no detector is wired.
	DetectorPrefilter bool

	// UseDetector, when true, signals the higher-level hush package to
	// auto-wire the embedded v2 NER detector. Implies ModelOff=true (the
	// v1 sequence classifier is not loaded). pkg/scanner itself does not
	// act on this field — see hush.New.
	UseDetector bool
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
	opts     Options
	scorer   Scorer
	detector Detector
	closer   func() error
}

// UseDetector switches the scanner to v2 NER mode. When set, the regex
// extractor and Scorer are bypassed; the detector emits spans directly.
// Pass nil to revert to the regex+scorer pipeline.
func (s *Scanner) UseDetector(d Detector) {
	s.detector = d
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
//
// When UseDetector has installed an NER detector (v2 path), the regex
// extractor and Scorer are bypassed and the detector emits spans
// directly. Otherwise the legacy regex + Scorer pipeline runs.
func (s *Scanner) ScanString(text string) ([]Finding, error) {
	if s.detector != nil {
		if s.opts.DetectorPrefilter {
			return scanWithDetectorPrefilter(text, s.detector, s.opts.MinConfidence, s.opts.EntropyThreshold)
		}
		return scanWithDetector(text, s.detector, s.opts.MinConfidence)
	}
	return Scan(text, s.opts.MinConfidence, s.opts.EntropyThreshold, s.opts.CtxChars, s.scorer)
}

// scanWithDetector runs an NER detector over text and maps Spans into
// Findings using the same JSON shape as the regex pipeline.
func scanWithDetector(text string, d Detector, minConfidence float64) ([]Finding, error) {
	spans, err := d.Detect(text)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, nil
	}
	out := make([]Finding, 0, len(spans))
	for _, sp := range spans {
		conf := float64(sp.Score)
		if conf < minConfidence {
			continue
		}
		start, end := sp.Start, sp.End
		if start < 0 {
			start = 0
		}
		if end > len(text) {
			end = len(text)
		}
		if end < start {
			end = start
		}
		raw := text[start:end]
		// Compute line/column for the start offset.
		line, col := lineColumn(text, start)
		out = append(out, Finding{
			Line:       line,
			Column:     col,
			Rule:       sp.Type,
			Span:       raw,
			Redacted:   extractor.Redact(raw),
			Start:      start,
			End:        end,
			Confidence: conf,
		})
	}
	return out, nil
}

// lineColumn returns the 1-based line and column for the given byte offset.
func lineColumn(text string, off int) (int, int) {
	if off > len(text) {
		off = len(text)
	}
	line, col := 1, 1
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// streamChunkSize is the per-chunk read size for ScanReader and the
// large-file path. 1 MB chosen to amortize tokenizer overhead while
// keeping peak RSS bounded.
const streamChunkSize = 1 << 20

// streamChunkOverlap is the number of bytes carried between adjacent
// chunks so a secret straddling a boundary still appears intact in one
// chunk.
const streamChunkOverlap = 4 << 10

// streamMaxChunks caps total chunks scanned per ScanReader call. At
// streamChunkSize=1 MB this is 200 MB of streaming work; beyond that
// the worker bails with a stderr note rather than spending unbounded
// time on a pathological input. Mirrors plan #12.
const streamMaxChunks = 200

// ScanReader scans an arbitrarily large reader without slurping the full
// stream into memory. As of v0.1.11 it reads in 1 MB chunks with a 4 KB
// trailing overlap and dedupes findings whose absolute offset matches
// across chunks. Constant memory regardless of input size (#15).
func (s *Scanner) ScanReader(r io.Reader) ([]Finding, error) {
	br := bufio.NewReaderSize(r, streamChunkSize+streamChunkOverlap)
	var (
		base   int
		carry  []byte
		out    []Finding
		seen   = make(map[int64]struct{}, 256)
		chunkN int
	)
	for {
		if chunkN >= streamMaxChunks {
			fmt.Fprintf(os.Stderr, "hush: ScanReader: stopping after %d chunks (%.0f MB) on this stream\n", chunkN, float64(chunkN)*float64(streamChunkSize)/1024/1024)
			break
		}
		chunkN++
		buf := make([]byte, streamChunkSize)
		n, err := io.ReadFull(br, buf)
		if n == 0 && err == io.EOF {
			break
		}
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		buf = buf[:n]
		// Prepend carry from previous chunk.
		chunkStart := base - len(carry)
		var chunk []byte
		if len(carry) == 0 {
			chunk = buf
		} else {
			chunk = append(carry, buf...)
		}
		findings, scanErr := s.ScanString(string(chunk))
		if scanErr != nil {
			return nil, scanErr
		}
		for _, f := range findings {
			absStart := chunkStart + f.Start
			absEnd := chunkStart + f.End
			key := int64(absStart)<<32 | int64(absEnd)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			f.Start = absStart
			f.End = absEnd
			out = append(out, f)
		}
		// Advance base past the buf, save the trailing overlap as the
		// next carry.
		base += n
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if len(buf) > streamChunkOverlap {
			carry = append(carry[:0], buf[len(buf)-streamChunkOverlap:]...)
		} else {
			carry = append(carry[:0], buf...)
		}
	}
	return out, nil
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
