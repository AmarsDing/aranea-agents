package biz

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// sanitizeInboxRelPath rejects path escape and returns a single path element
// (or a slash-joined relative path without ".."). Empty / absolute → "".
func sanitizeInboxRelPath(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\\", "/")
	if s == "" || path.IsAbs(s) {
		return ""
	}
	s = path.Clean(s)
	if s == "." || s == ".." || strings.HasPrefix(s, "../") {
		return ""
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." || seg == "" {
			return ""
		}
	}
	return s
}

func teamDefinitionAgentKeys(t Team) []string {
	if strings.TrimSpace(t.DefinitionJSON) == "" {
		return nil
	}
	var def struct {
		Members []struct {
			AgentID string `json:"agent_id"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(t.DefinitionJSON), &def); err != nil {
		return nil
	}
	out := make([]string, 0, len(def.Members))
	seen := map[string]struct{}{}
	for _, m := range def.Members {
		k := strings.TrimSpace(m.AgentID)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func (d *SpiritDelivery) materializeUpstreamInbox(ctx context.Context, downstream Team) {
	if d == nil || d.inboxFS == nil || len(downstream.DependsOn) == 0 {
		return
	}
	teams, err := d.teamUC.ListBySpiritSessionID(ctx, downstream.SpiritSessionID)
	if err != nil {
		d.lg.Warn("物化 inbox 查询团队失败",
			loggateway.StepID("spirit.inbox.list"),
			loggateway.Err(err),
		)
		return
	}
	byDag := make(map[string]Team, len(teams))
	for _, t := range teams {
		if t.DagNodeID != "" {
			byDag[t.DagNodeID] = t
		}
	}
	destKeys := teamDefinitionAgentKeys(downstream)
	if len(destKeys) == 0 {
		return
	}
	for _, depID := range downstream.DependsOn {
		upstream, ok := byDag[depID]
		if !ok || upstream.Status != TeamStatusCompleted {
			continue
		}
		ref, refOK := d.readDeliverableRef(upstream)
		if !refOK {
			continue
		}
		srcKeys := teamDefinitionAgentKeys(upstream)
		for _, art := range ref.Artifacts {
			if !art.IsBulkPointer() {
				continue
			}
			rel := strings.TrimSpace(art.RelPath)
			if rel == "" {
				continue
			}
			destName := path.Base(rel)
			if destName == "" || destName == "." || destName == "/" {
				continue
			}
			if copyErr := d.inboxFS.MaterializeFile(ctx, InboxCopySpec{
				SrcAgentKeys:   srcKeys,
				DestAgentKeys:  destKeys,
				UpstreamTeamID: upstream.ID,
				RelPath:        rel,
				DestName:       destName,
			}); copyErr != nil {
				d.lg.Warn("物化 inbox 附件失败，跳过该文件",
					loggateway.StepID("spirit.inbox.copy"),
					loggateway.Str("upstream_team_id", upstream.ID),
					loggateway.Str("rel_path", rel),
					loggateway.Err(copyErr),
				)
			}
		}
	}
}
