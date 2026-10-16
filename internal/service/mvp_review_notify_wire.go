package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/mvp"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

// WireMVPReviewChannelNotify registers native IM ChannelEnqueue for pending-review
// notify (Channel ≠ Publish). Call once when ChannelUsecase is ready.
// Delivery goes through EnqueueOutboundDelivery → worker → platform SendText.
func WireMVPReviewChannelNotify(channels *biz.ChannelUsecase) {
	if channels == nil {
		return
	}
	mvp.ConfigureChannelEnqueue(func(ctx context.Context, channelID, platform, recipient, text, idempotencyKey string) error {
		channelID = strings.TrimSpace(channelID)
		recipient = strings.TrimSpace(recipient)
		text = strings.TrimSpace(text)
		if channelID == "" || recipient == "" || text == "" {
			return nil
		}
		plat := strings.TrimSpace(platform)
		if plat == "" {
			ch, err := channels.Get(ctx, channelID)
			if err != nil {
				return err
			}
			plat = biz.ChannelTypeFromConfig(ch.ConfigJSON)
		}
		if plat == "" {
			plat = "feishu"
		}
		_, _, err := channels.EnqueueOutboundDelivery(ctx, channelID, biz.ChannelOutboundPayload{
			Kind:           biz.ChannelOutboundTextKind,
			Platform:       plat,
			Recipient:      recipient,
			Text:           text,
			IdempotencyKey: strings.TrimSpace(idempotencyKey),
		})
		return err
	})
}

// WireMVPWebResearchSearcher installs DefaultSearcher and optional platform
// system_settings loader for MVP search ingest (hits[] optional).
func WireMVPWebResearchSearcher(repo biz.SystemSettingRepo) {
	if repo != nil {
		mvp.ConfigureWebResearchLoader(func(ctx context.Context) (webresearchpkg.Config, bool) {
			s, err := repo.GetWebResearch(ctx)
			if err != nil {
				return webresearchpkg.Config{}, false
			}
			pf := &webresearchpkg.PlatformFields{
				HasAPIKey:   s.HasAPIKey || strings.TrimSpace(s.APIKey) != "",
				APIKey:      strings.TrimSpace(s.APIKey),
				Provider:    strings.TrimSpace(s.Provider),
				MaxResults:  s.MaxResults,
				FetchTop:    0, // ingest: titles/snippets only
				SearchDepth: strings.TrimSpace(s.SearchDepth),
				TimeoutSec:  s.TimeoutSec,
				HTTPProxy:   strings.TrimSpace(s.HTTPProxy),
			}
			cfg := webresearchpkg.ResolveConfig(nil, pf)
			if !cfg.Ready() {
				return webresearchpkg.Config{}, false
			}
			cfg.FetchTop = 0
			cfg.IncludeRawContent = false
			return cfg, true
		})
	}
	mvp.ConfigureSearcher(mvp.DefaultSearcher())
}
