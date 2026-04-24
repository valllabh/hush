# GitLab CI

## Pipeline

`.gitlab-ci.yml`:

```yaml
hush:
  image: ghcr.io/valllabh/hush:latest
  stage: test
  script:
    - hush scan . --format json --fail-on-finding > hush.json
  artifacts:
    when: always
    reports:
      sast: hush.json
    paths: [hush.json]
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

GitLab renders the SAST report on the MR page. Findings above the
confidence threshold block merge.

## MR comment bot

Add [danger](https://danger.systems/) or a simple curl to
`/api/v4/projects/:id/merge_requests/:iid/notes` with the formatted
findings.
