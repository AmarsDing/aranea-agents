# Artifact Storage — Usage & Configuration

> Sprint 5 · T34 · Added 2026-05-17

Aranea-Agents supports saving and retrieving binary **artifacts** (files produced by agents during a session) via the Artifact Service.

---

## Architecture

```
Frontend (base64 HTTP)
    │
    ▼
internal/service/artifact.go   ← Kratos HTTP handler
    │
    ▼
internal/biz/artifact.go       ← ArtifactUsecase + ArtifactRepo interface
    │
    ▼
internal/data/artifactfs/      ← local filesystem implementation
    │  repo.go                 ← versioned storage in {artifact.dir}/<session>/<name>/v<N>.bin
    │  meta.json               ← per-artifact metadata sidecar
    │
    ▼
internal/artifact/trpc/        ← adapter for trpc-agent-go runtime
```

---

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/artifacts` | Upload (base64-encoded) |
| `GET`  | `/v1/artifacts/{id}` | Download (base64-encoded) |
| `GET`  | `/v1/artifacts?session_id=…` | List metadata for a session |
| `DELETE` | `/v1/artifacts/{id}` | Delete all versions |

### Upload request body

```json
{
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "data_base64": "<standard-base64>"
}
```

Size limit: **50 MB** per artifact.

### Response — ArtifactMeta

```json
{
  "id": "a9f3c1d2e4b5",
  "session_id": "sess-abc123",
  "name": "report.pdf",
  "mime_type": "application/pdf",
  "size": 204800,
  "sha256": "e3b0c44298fc…",
  "storage_kind": "fs",
  "storage_uri": "/data/artifacts/sess-abc123/report.pdf/v1.bin",
  "version": 1,
  "created_at": "2026-05-17T10:30:00Z"
}
```

---

## Storage Configuration

| Config key | Default | Description |
|------------|---------|-------------|
| `data.artifact.dir` | `./data/artifacts` | Root directory for artifact files |

Each artifact is stored as:
```
{artifact.dir}/{session_id}/{name}/v{version}.bin
{artifact.dir}/{session_id}/{name}/meta.json
```

Uploading a file with the same `session_id` + `name` creates a new version (`v2.bin`, `v3.bin`, …).

---

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `aranea_artifact_upload_bytes_total` | Counter | — | Total bytes uploaded |
| `aranea_artifact_download_bytes_total` | Counter | — | Total bytes downloaded |
| `aranea_artifact_storage_bytes` | Gauge | — | Total bytes on disk (approximated) |

---

## Agent Integration

Agents access artifacts through the `trpc-agent-go` artifact service:

```go
// inside a tool or agent run
svc := ctx.Value(artifact.ServiceKey{}).(artifact.Service)

// save
version, err := svc.SaveArtifact(ctx, sessionInfo, "output.csv", &artifact.Artifact{
    Data:     csvBytes,
    MimeType: "text/csv",
})

// load latest version
a, err := svc.LoadArtifact(ctx, sessionInfo, "output.csv", nil)
```

---

## Known Limitations

- Binary storage uses the local filesystem only. S3/GCS backends are planned for S6.
- Artifacts are not replicated across nodes. Use a shared volume in multi-instance deployments.
- `data_base64` encoding adds ~33% overhead; large files (> 10 MB) should prefer chunked streaming (planned).
