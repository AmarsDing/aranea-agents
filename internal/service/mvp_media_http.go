package service

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/connector/ingest/search"
	"aranea-agents/internal/mvp"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterMVPMediaRoutes mounts MVP content/review/publish/commerce HTTP APIs.
// Channel ≠ Publish: these are not IM channel routes.
func RegisterMVPMediaRoutes(srv *kratoshttp.Server) {
	if srv == nil {
		return
	}
	r := srv.Route("/")
	r.GET("/api/v1/mvp/content", handleMVPContentList)
	r.POST("/api/v1/mvp/content", handleMVPContentCreate)
	r.GET("/api/v1/mvp/content/{id}", handleMVPContentGet)
	r.POST("/api/v1/mvp/content/{id}/select", handleMVPContentSelect)

	r.POST("/api/v1/mvp/ingest/rss", handleMVPIngestRSS)
	r.POST("/api/v1/mvp/ingest/search", handleMVPIngestSearch)

	r.GET("/api/v1/mvp/review/pending", handleMVPReviewPending)
	r.POST("/api/v1/mvp/review", handleMVPReviewEnqueue)
	r.POST("/api/v1/mvp/review/{id}/approve", handleMVPReviewApprove)
	r.POST("/api/v1/mvp/review/{id}/reject", handleMVPReviewReject)

	r.GET("/api/v1/mvp/publish/jobs", handleMVPPublishList)
	r.POST("/api/v1/mvp/publish/from-review/{id}", handleMVPPublishFromReview)
	r.POST("/api/v1/mvp/publish/process", handleMVPPublishProcess)
	r.GET("/api/v1/mvp/publish/jobs/{id}", handleMVPPublishGet)
	r.GET("/api/v1/mvp/publish/jobs/{id}/files/{name}", handleMVPPublishFile)

	r.GET("/api/v1/mvp/opportunities", handleMVPOppList)
	r.POST("/api/v1/mvp/opportunities", handleMVPOppCreate)
	r.POST("/api/v1/mvp/opportunities/{id}/ack", handleMVPOppAck)

	r.GET("/api/v1/mvp/sourcing", handleMVPSourcingList)
	r.POST("/api/v1/mvp/sourcing", handleMVPSourcingCreate)
	r.POST("/api/v1/mvp/sourcing/{id}/status", handleMVPSourcingStatus)
}

func writeJSON(ctx kratoshttp.Context, code int, v any) error {
	ctx.Response().Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx.Response().WriteHeader(code)
	return json.NewEncoder(ctx.Response()).Encode(v)
}

func readJSON(ctx kratoshttp.Context, dst any) error {
	defer ctx.Request().Body.Close()
	return json.NewDecoder(ctx.Request().Body).Decode(dst)
}

func handleMVPContentList(ctx kratoshttp.Context) error {
	rt := mvp.Ensure()
	q := ctx.Request().URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	items, total, err := rt.Content.List(content.ListFilter{
		WorkspaceID: q.Get("workspace_id"),
		SourceType:  q.Get("source_type"),
		Status:      q.Get("status"),
		Limit:       limit,
	})
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": items, "total": total})
}

func handleMVPContentCreate(ctx kratoshttp.Context) error {
	var in content.Item
	if err := readJSON(ctx, &in); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	out, dup, err := mvp.Ensure().Content.CreateFromIngest(in)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"item": out, "deduped": dup})
}

func handleMVPContentGet(ctx kratoshttp.Context) error {
	id := ctx.Vars().Get("id")
	it, err := mvp.Ensure().Content.Get(id)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, it)
}

func handleMVPContentSelect(ctx kratoshttp.Context) error {
	it, err := mvp.Ensure().Content.SelectForProduction(ctx.Vars().Get("id"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, it)
}

type ingestRSSBody struct {
	FeedURL     string   `json:"feed_url"`
	FeedURLs    []string `json:"feed_urls"`
	WorkspaceID string   `json:"workspace_id"`
}

func handleMVPIngestRSS(ctx kratoshttp.Context) error {
	var body ingestRSSBody
	if err := readJSON(ctx, &body); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	urls := append([]string{}, body.FeedURLs...)
	if strings.TrimSpace(body.FeedURL) != "" {
		urls = append(urls, body.FeedURL)
	}
	if len(urls) == 0 {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": "feed_url or feed_urls required"})
	}
	res, err := mvp.IngestRSSFeeds(ctx.Request().Context(), urls, body.WorkspaceID)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
	}
	return writeJSON(ctx, nethttp.StatusOK, res)
}

type ingestSearchBody struct {
	Query       string       `json:"query"`
	WorkspaceID string       `json:"workspace_id"`
	Hits        []search.Hit `json:"hits"`
}

func handleMVPIngestSearch(ctx kratoshttp.Context) error {
	var body ingestSearchBody
	if err := readJSON(ctx, &body); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	res, err := mvp.IngestSearchHits(ctx.Request().Context(), body.WorkspaceID, body.Query, body.Hits)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, res)
}

func handleMVPReviewPending(ctx kratoshttp.Context) error {
	items, total, err := mvp.Ensure().Review.ListPending(50, 0)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": items, "total": total})
}

func handleMVPReviewEnqueue(ctx kratoshttp.Context) error {
	var in review.Task
	if err := readJSON(ctx, &in); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	out, err := mvp.Ensure().Review.Enqueue(in)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	// Keep content status in sync when possible.
	if in.ContentItemID != "" {
		_, _ = mvp.Ensure().Content.MarkInReview(in.ContentItemID)
	}
	return writeJSON(ctx, nethttp.StatusOK, out)
}

type reviewDecideBody struct {
	ReviewerID string `json:"reviewer_id"`
	Note       string `json:"note"`
	Platform   string `json:"platform"`
}

func handleMVPReviewApprove(ctx kratoshttp.Context) error {
	var body reviewDecideBody
	_ = readJSON(ctx, &body)
	id := ctx.Vars().Get("id")
	rt := mvp.Ensure()
	task, err := rt.Review.Approve(id, body.ReviewerID, body.Note)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if task.ContentItemID != "" {
		_, _ = rt.Content.MarkApproved(task.ContentItemID)
	}
	platform := body.Platform
	if platform == "" {
		platform = "generic"
	}
	job, err := rt.Publish.CreateFromApprovedReview(task, platform)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	_, _ = rt.Publish.ProcessNext(1)
	job, _ = rt.Publish.Get(job.ID)
	_ = mvp.AfterReviewDecision(task)
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{
		"review":      task,
		"publish_job": job,
		"note":        "packaged ≠ posted_to_platform; download manifest.json/body.md and paste manually",
	})
}

func handleMVPReviewReject(ctx kratoshttp.Context) error {
	var body reviewDecideBody
	_ = readJSON(ctx, &body)
	task, err := mvp.Ensure().Review.Reject(ctx.Vars().Get("id"), body.ReviewerID, body.Note)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if task.ContentItemID != "" {
		_, _ = mvp.Ensure().Content.MarkRejected(task.ContentItemID)
	}
	_ = mvp.AfterReviewDecision(task)
	return writeJSON(ctx, nethttp.StatusOK, task)
}

func handleMVPPublishList(ctx kratoshttp.Context) error {
	status := ctx.Request().URL.Query().Get("status")
	if status == "" {
		status = "packaged"
	}
	jobs, err := mvp.Ensure().Publish.List(status, 50)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": jobs})
}

func handleMVPPublishFromReview(ctx kratoshttp.Context) error {
	rt := mvp.Ensure()
	task, err := rt.Review.Get(ctx.Vars().Get("id"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusNotFound, map[string]string{"error": err.Error()})
	}
	platform := ctx.Request().URL.Query().Get("platform")
	job, err := rt.Publish.CreateFromApprovedReview(task, platform)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, job)
}

func handleMVPPublishProcess(ctx kratoshttp.Context) error {
	n, err := mvp.Ensure().Publish.ProcessNext(20)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"processed": n})
}

func handleMVPPublishGet(ctx kratoshttp.Context) error {
	job, err := mvp.Ensure().Publish.Get(ctx.Vars().Get("id"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusNotFound, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, job)
}

func handleMVPPublishFile(ctx kratoshttp.Context) error {
	job, err := mvp.Ensure().Publish.Get(ctx.Vars().Get("id"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusNotFound, map[string]string{"error": err.Error()})
	}
	path, err := mvp.PackageFilePath(job, ctx.Vars().Get("name"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	f, err := os.Open(path)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusNotFound, map[string]string{"error": "package file missing: " + err.Error()})
	}
	defer f.Close()
	name := filepath.Base(path)
	ctype := "application/octet-stream"
	switch name {
	case "manifest.json":
		ctype = "application/json; charset=utf-8"
	case "body.md":
		ctype = "text/markdown; charset=utf-8"
	}
	w := ctx.Response()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Aranea-Package-Semantics", "packaged_not_posted_to_platform")
	w.WriteHeader(nethttp.StatusOK)
	_, copyErr := io.Copy(w, f)
	return copyErr
}

func handleMVPOppList(ctx kratoshttp.Context) error {
	q := ctx.Request().URL.Query()
	items, total, err := mvp.Ensure().Opportunity.List(q.Get("audience"), q.Get("status"), 50, 0)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": items, "total": total})
}

func handleMVPOppCreate(ctx kratoshttp.Context) error {
	var in commerce.Opportunity
	if err := readJSON(ctx, &in); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	out, err := mvp.Ensure().Opportunity.Create(in)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, out)
}

func handleMVPOppAck(ctx kratoshttp.Context) error {
	out, err := mvp.Ensure().Opportunity.Acknowledge(ctx.Vars().Get("id"))
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, out)
}

func handleMVPSourcingList(ctx kratoshttp.Context) error {
	q := ctx.Request().URL.Query()
	oppID := q.Get("opportunity_id")
	if oppID != "" {
		items, err := mvp.Ensure().Sourcing.ListByOpportunity(oppID)
		if err != nil {
			return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": items, "total": len(items)})
	}
	items, total, err := mvp.Ensure().Sourcing.List(50, 0)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, map[string]any{"items": items, "total": total})
}

func handleMVPSourcingCreate(ctx kratoshttp.Context) error {
	var in commerce.SupplierCandidate
	if err := readJSON(ctx, &in); err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	out, err := mvp.Ensure().Sourcing.Create(in)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, out)
}

type sourcingStatusBody struct {
	Status string `json:"status"`
}

func handleMVPSourcingStatus(ctx kratoshttp.Context) error {
	var body sourcingStatusBody
	_ = readJSON(ctx, &body)
	out, err := mvp.Ensure().Sourcing.SetStatus(ctx.Vars().Get("id"), body.Status)
	if err != nil {
		return writeJSON(ctx, nethttp.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return writeJSON(ctx, nethttp.StatusOK, out)
}
