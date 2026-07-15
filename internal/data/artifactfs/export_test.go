package artifactfs

// ResolveBinPath exposes resolveBinPath for containment unit tests (C-02).
func ResolveBinPath(r *FSArtifactRepo, meta ArtifactMeta) string {
	return r.resolveBinPath(meta)
}

// ArtifactMeta is the exported alias of the internal metadata sidecar shape.
type ArtifactMeta = artifactMeta
