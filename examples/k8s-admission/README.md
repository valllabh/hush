# Kubernetes admission controller

Block `ConfigMap` and `Secret` applies that contain secrets stored in
plaintext in the wrong field. Surprisingly common: someone pastes a DB
password into a ConfigMap instead of a Secret.

## Pattern

```
kubectl apply  ->  api server  ->  ValidatingWebhook  ->  hush
```

## Options

- Write a dedicated webhook in Go using [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime),
  call `scanner.ScanString` on each field value.
- Use [Kyverno](https://kyverno.io/) with a `verifyImages` style rule
  that invokes hush as an external validator.
- Use [OPA Gatekeeper](https://open-policy-agent.github.io/gatekeeper/)
  and call hush from a sidecar over HTTP.

## Webhook skeleton (Go)

```go
func (h *HushAdmitter) Handle(ctx context.Context, req admission.Request) admission.Response {
    var obj corev1.ConfigMap
    json.Unmarshal(req.Object.Raw, &obj)
    for _, v := range obj.Data {
        if findings, _ := h.Scanner.ScanString(v); len(findings) > 0 {
            return admission.Denied(fmt.Sprintf("secret detected: %s", findings[0].Rule))
        }
    }
    return admission.Allowed("")
}
```

Deploy as a `ValidatingWebhookConfiguration` scoped to ConfigMaps and
Secrets.
