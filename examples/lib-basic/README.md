# lib-basic

The smallest possible hush library usage. Reads text from stdin, prints
findings (secrets and PII). One import. Works out of the box.

## Run

```
go run . < sample.txt
```

Expected output:

```
line 2 col 11  secret   AKI**************PLE  confidence=1.00
line 3 col 10  pii      val*****************com  confidence=1.00
line 4 col 8   pii      415********71  confidence=1.00
```

(Exit code is 1 when findings are present.)

## The code

```go
import "github.com/valllabh/hush"

s, _ := hush.New(hush.Options{
    UseDetector:       true, // v2 NER detector
    DetectorPrefilter: true, // skip model on clean text
    MinConfidence:     0.5,
})
defer s.Close()

findings, _ := s.ScanReader(os.Stdin)
for _, f := range findings {
    fmt.Printf("line %d col %d  %s  %s  confidence=%.2f\n",
        f.Line, f.Column, f.Rule, f.Redacted, f.Confidence)
}
```

`f.Rule` is the class: `secret` or `pii`. The other library examples
build on this shape.
