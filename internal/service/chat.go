package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/legacychat"
	"aranea-agents/internal/team"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/structpb"
)

// ChatService implements kratos chat.v1 unary RPCs via legacy upstream proxy when LEGACY_REST_ORIGIN is set.
type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	mu          sync.RWMutex
	up          *url.URL
	proxy       *httputil.ReverseProxy
	client      *http.Client
	llmHTTP     *http.Client
	teamSSE     *biz.TeamRunEventBroker
	teams       biz.TeamRepository
	teamsNative *team.Runner
	usage       *biz.UsageUsecase
	sessions    *biz.SessionUsecase
	agents      biz.AgentRepository
	agentsUC    *biz.AgentUsecase
	llmCatalog  *biz.LlmProviderModelUsecase
}

// NewChatService builds a chat façade (LEGACY_REST_ORIGIN → legacy /api/v1/chat/* until fully in-process execution).
func NewChatService(
	broker *biz.TeamRunEventBroker,
	teams biz.TeamRepository,
	teamsNative *team.Runner,
	usage *biz.UsageUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	llmCatalog *biz.LlmProviderModelUsecase,
) *ChatService {
	s := &ChatService{
		client:      &http.Client{Timeout: 600 * time.Second},
		llmHTTP:     &http.Client{Timeout: 300 * time.Second},
		teamSSE:     broker,
		teams:       teams,
		teamsNative: teamsNative,
		usage:       usage,
		sessions:    sessions,
		agents:      agents,
		agentsUC:    agentsUC,
		llmCatalog:  llmCatalog,
	}
	s.refreshUpstream()
	return s
}

func (s *ChatService) refreshUpstream() {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN"))
	if raw == "" {
		s.up = nil
		s.proxy = nil
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		s.up = nil
		s.proxy = nil
		return
	}
	s.up = u
	s.proxy = legacychat.LegacyChatReverseProxy(u)
}

func (s *ChatService) upstreamRoot() (*url.URL, *httputil.ReverseProxy) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.up, s.proxy
}

// ProxyStream attaches POST /v1/chat/messages/stream (SSE passthrough).
func (s *ChatService) ProxyStream(ctx khttp.Context) error {
	_, proxy := s.upstreamRoot()
	if proxy == nil {
		return s.proxyNativeStream(ctx)
	}
	proxy.ServeHTTP(ctx.Response(), ctx.Request())
	return nil
}

// SendChatMessage implements unary POST /v1/chat/messages → legacy /api/v1/chat/messages.
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	baseRaw := strings.TrimRight(strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN")), "/")
	if baseRaw == "" {
		return s.nativeSendChatMessage(ctx, req)
	}

	body := map[string]any{
		"session_id": strings.TrimSpace(req.GetSessionId()),
		"content":    strings.TrimSpace(req.GetContent()),
	}
	if ak := strings.TrimSpace(req.GetAgentKey()); ak != "" {
		body["agent_key"] = ak
	}
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		body["team_id"] = tid
	}
	if opts := req.GetOptions(); opts != nil {
		om := map[string]any{}
		if dm := opts.GetDialogMode(); dm != "" {
			om["dialog_mode"] = dm
		}
		if p := opts.GetProvider(); p != "" {
			om["provider"] = p
		}
		if m := opts.GetModel(); m != "" {
			om["model"] = m
		}
		if len(opts.Attachments) > 0 {
			var atts []map[string]any
			for _, a := range opts.Attachments {
				atts = append(atts, map[string]any{"id": strings.TrimSpace(a.GetId())})
			}
			om["attachments"] = atts
		}
		if len(om) > 0 {
			body["options"] = om
		}
	}

	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := baseRaw + legacychat.MessagesPath
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(rawBody)))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, kerrors.BadRequest("CHAT_UPSTREAM",
			fmt.Sprintf("legacy chat POST %s: %s %s", endpoint, resp.Status, strings.TrimSpace(string(respBody))))
	}

	var decoded map[string]any
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}

	out := &chatv1.SendChatMessageResponse{}
	um := pickMap(decoded["user_message"])
	am := pickMap(decoded["agent_message"])
	if um != nil {
		st, err := structpb.NewStruct(um)
		if err != nil {
			return nil, err
		}
		out.UserMessage = st
	}
	if am != nil {
		st, err := structpb.NewStruct(am)
		if err != nil {
			return nil, err
		}
		out.AgentMessage = st
	}
	recordChatIngressUsage(ctx, s.usage, req, am, false)
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		biz.HintTeamRunSSE(ctx, s.teamSSE, s.teams, tid)
	}
	return out, nil
}

func pickMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	return m
}

// GetChatOptions implements GET /v1/chat/options.
func (s *ChatService) GetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	baseRaw := strings.TrimRight(strings.TrimSpace(os.Getenv("LEGACY_REST_ORIGIN")), "/")
	if baseRaw == "" {
		return s.nativeGetChatOptions(ctx, req)
	}

	rawURL := baseRaw + legacychat.LegacyRoutePrefix + "/options"
	if typed := strings.TrimSpace(req.GetType()); typed != "" {
		rawURL = rawURL + "?type=" + url.QueryEscape(typed)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, kerrors.BadRequest("CHAT_UPSTREAM",
			fmt.Sprintf("legacy chat GET %s: %s %s", rawURL, resp.Status, strings.TrimSpace(string(respBody))))
	}

	var decoded struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	out := &chatv1.GetChatOptionsResponse{}
	for _, it := range decoded.Items {
		out.Items = append(out.Items, rowToProtoChatOption(it))
	}
	return out, nil
}

func rowToProtoChatOption(m map[string]any) *chatv1.ChatOption {
	c := &chatv1.ChatOption{}
	if v, ok := m["type"].(string); ok {
		c.Type = v
	}
	if v, ok := m["key"].(string); ok {
		c.Key = v
	}
	if v, ok := m["label"].(string); ok {
		c.Label = v
	}
	if v, ok := m["enabled"].(bool); ok {
		c.Enabled = v
	}
	switch so := m["sort_order"].(type) {
	case float64:
		c.SortOrder = int32(so)
	case int:
		c.SortOrder = int32(so)
	case int64:
		c.SortOrder = int32(so)
	}
	if v, ok := m["metadata_json"].(string); ok {
		c.MetadataJson = v
	}
	return c
}
