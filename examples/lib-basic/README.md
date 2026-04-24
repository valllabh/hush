# lib-basic

The smallest possible hush library usage. Reads text from stdin, prints
findings. One import. Works out of the box.

## Run

```
go run . < sample.txt
```

Expected output:

```
line 2 col 11  aws_access_key_id  confidence=1.00
```

(Exit code is 1 when findings are present.)

## The code

```go
import "github.com/valllabh/hush"

s, _ := hush.New(hush.Options{MinConfidence: 0.9})
defer s.Close()

findings, _ := s.ScanReader(os.Stdin)
for _, f := range findings {
    fmt.Printf("line %d  %s  confidence=%.2f\n", f.Line, f.Rule, f.Confidence)
}
```

One import, clean product namespace. Start here. The other library
examples build on this shape.
