# Docker image scan

Scan the filesystem of a built image for secrets baked into layers.
Catches the "we committed `.env` into the Dockerfile COPY" class of leak.

## Inline (single command)

```
docker save myimage:latest | hush scan --format json -
```

## Mount and scan

```
container=$(docker create myimage:latest)
docker export $container | tar -x -C /tmp/img
hush scan /tmp/img --fail-on-finding
docker rm $container
```

## As a CI step

```yaml
- name: Build image
  run: docker build -t app:${{ github.sha }} .
- name: Scan layers
  run: |
    docker save app:${{ github.sha }} -o image.tar
    hush scan image.tar --fail-on-finding
```

Use alongside [dive](https://github.com/wagoodman/dive) or
[trivy](https://github.com/aquasecurity/trivy). Hush focuses on secrets;
the others cover CVEs and misconfigs.
