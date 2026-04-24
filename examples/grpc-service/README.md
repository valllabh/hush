# gRPC service

Same idea as [rest-server](../rest-server/) but as a gRPC service, for
polyglot environments that already speak gRPC.

## Proto

```proto
syntax = "proto3";
package hush.v1;

service Hush {
  rpc Scan(ScanRequest) returns (ScanResponse);
  rpc ScanStream(stream ScanChunk) returns (stream Finding);
}

message ScanRequest { string text = 1; float min_confidence = 2; }
message Finding { string rule = 1; uint32 line = 2; uint32 column = 3; float score = 4; }
message ScanResponse { repeated Finding findings = 1; }
message ScanChunk { bytes data = 1; }
```

## Snippet

```go
type server struct { s *scanner.Scanner }

func (srv *server) Scan(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
    findings, _ := srv.s.ScanString(req.Text)
    out := &pb.ScanResponse{}
    for _, f := range findings { out.Findings = append(out.Findings, toPb(f)) }
    return out, nil
}
```

The streaming variant is useful for log pipelines that want constant
memory.
