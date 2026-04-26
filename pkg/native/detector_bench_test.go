package native

import "testing"

// benchDocCodeLike is a ~4 KB code-shaped blob with a few credentials
// sprinkled in. Used by BenchmarkDetectorDetect to measure end-to-end
// latency on a realistic input. Spec target: < 50 ms on Apple Silicon,
// < 200 ms on x86. The benchmark records ns/op; it does NOT fail the
// build on missing target.
var benchDocCodeLike = `# config.yaml — service deployment
service:
  name: payment-gateway
  region: us-east-1
  replicas: 4
  image: registry.example.com/payments/api:1.42.0
  env:
    - name: DATABASE_URL
      value: postgres://app:hunter2@db.internal:5432/payments
    - name: REDIS_URL
      value: redis://cache.internal:6379/0
    - name: AWS_ACCESS_KEY_ID
      value: AKIAIOSFODNN7EXAMPLE
    - name: AWS_SECRET_ACCESS_KEY
      value: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
    - name: STRIPE_SECRET
      value: ` + "sk_" + "live_" + "TESTONLYTESTONLYTESTONLYTESTONLY" + `
    - name: SENTRY_DSN
      value: https://abc123@o0.ingest.sentry.io/0
metadata:
  owner: payments-team@example.com
  pager: +1-415-555-2671
  notes: |
    Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
    tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim
    veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex
    ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate
    velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint
    occaecat cupidatat non proident, sunt in culpa qui officia deserunt
    mollit anim id est laborum.

    Sed ut perspiciatis unde omnis iste natus error sit voluptatem
    accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae
    ab illo inventore veritatis et quasi architecto beatae vitae dicta
    sunt explicabo. Nemo enim ipsam voluptatem quia voluptas sit aspernatur
    aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione
    voluptatem sequi nesciunt.

    UUIDs to ignore: 550e8400-e29b-41d4-a716-446655440000 and
    f47ac10b-58cc-4372-a567-0e02b2c3d479. SHA-256 fingerprint:
    e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.
healthcheck:
  path: /healthz
  interval: 10s
  timeout: 2s
secrets:
  github_token: ghp_TESTONLYTESTONLYTESTONLYTESTONLYTEST6
  internal_token: qx_internal_TESTONLYTESTONLYTESTONLYTESTONLYTEST
# end of config
`

func BenchmarkDetectorDetect(b *testing.B) {
	d, err := NewBundledDetector()
	if err != nil {
		b.Fatalf("NewBundledDetector: %v", err)
	}
	defer d.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		spans, err := d.Detect(benchDocCodeLike)
		if err != nil {
			b.Fatalf("Detect: %v", err)
		}
		_ = spans
	}
}
