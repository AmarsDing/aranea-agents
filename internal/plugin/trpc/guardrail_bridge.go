package plugintrpc

import (
	"context"
	"strings"
	"unicode"

	trpcguardrail "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail"
	trpcpromptinjection "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/promptinjection"
	trpcpromptreview "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/promptinjection/review"
	trpcunsafeintent "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/unsafeintent"
	trpcunsafereview "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/unsafeintent/review"
)

func normalizeInput(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func buildTranscriptInput(lastInput string, transcript []trpcpromptreview.TranscriptEntry) string {
	var b strings.Builder
	b.WriteString(normalizeInput(lastInput))
	for _, t := range transcript {
		b.WriteByte(' ')
		b.WriteString(normalizeInput(t.Content))
	}
	return b.String()
}

func buildUnsafeTranscriptInput(lastInput string, transcript []trpcunsafereview.TranscriptEntry) string {
	var b strings.Builder
	b.WriteString(normalizeInput(lastInput))
	for _, t := range transcript {
		b.WriteByte(' ')
		b.WriteString(normalizeInput(t.Content))
	}
	return b.String()
}

type ruleBasedPromptInjectionReviewer struct{}

var _ trpcpromptreview.Reviewer = (*ruleBasedPromptInjectionReviewer)(nil)

func (r *ruleBasedPromptInjectionReviewer) Review(_ context.Context, req *trpcpromptreview.Request) (*trpcpromptreview.Decision, error) {
	if req == nil {
		return &trpcpromptreview.Decision{Blocked: false}, nil
	}
	input := buildTranscriptInput(req.LastUserInput, req.Transcript)
	if cat, reason := detectPromptInjection(input); cat != "" {
		return &trpcpromptreview.Decision{
			Blocked:  true,
			Category: cat,
			Reason:   reason,
		}, nil
	}
	return &trpcpromptreview.Decision{Blocked: false}, nil
}

func detectPromptInjection(input string) (trpcpromptreview.Category, string) {
	normalized := normalizeInput(input)
	systemOverride := []string{
		"ignore previous instructions",
		"ignore all previous",
		"disregard your instructions",
		"forget your instructions",
		"ignore your instructions",
		"override system",
		"system prompt:",
		"new instructions:",
		"above instructions are",
		"real instructions are",
		"your real task is",
		"your actual task is",
		"ignore everything above",
		"ignore the above",
	}
	for _, p := range systemOverride {
		if strings.Contains(normalized, p) {
			return trpcpromptreview.CategorySystemOverride, "potential system override detected"
		}
	}

	policyBypass := []string{
		"bypass your safety",
		"bypass your rules",
		"bypass your guidelines",
		"ignore your rules",
		"ignore your guidelines",
		"do not follow your",
		"do not abide by",
		"jailbreak",
		"dan mode",
		"developer mode",
		"enable developer",
		"disable safety",
		"turn off safety",
		"remove restrictions",
		"ignore restrictions",
	}
	for _, p := range policyBypass {
		if strings.Contains(normalized, p) {
			return trpcpromptreview.CategoryPolicyBypass, "potential policy bypass detected"
		}
	}

	roleHijack := []string{
		"you are now",
		"pretend you are",
		"act as if you are",
		"simulate being",
		"from now on you are",
		"you have been",
		"switch to role",
		"roleplay as",
		"role play as",
	}
	for _, p := range roleHijack {
		if strings.Contains(normalized, p) {
			return trpcpromptreview.CategoryRoleHijack, "potential role hijack detected"
		}
	}

	exfil := []string{
		"repeat your instructions",
		"show your prompt",
		"reveal your system",
		"output your instructions",
		"print your instructions",
		"what are your instructions",
		"what is your system prompt",
	}
	for _, p := range exfil {
		if strings.Contains(normalized, p) {
			return trpcpromptreview.CategoryPromptExfiltration, "potential prompt exfiltration detected"
		}
	}

	toolMisuse := []string{
		"use tool to",
		"call function to",
		"execute command to",
		"run script to",
	}
	for _, p := range toolMisuse {
		if strings.Contains(normalized, p) {
			return trpcpromptreview.CategoryToolMisuseInduction, "potential tool misuse induction detected"
		}
	}

	return "", ""
}

type ruleBasedUnsafeIntentReviewer struct{}

var _ trpcunsafereview.Reviewer = (*ruleBasedUnsafeIntentReviewer)(nil)

func (r *ruleBasedUnsafeIntentReviewer) Review(_ context.Context, req *trpcunsafereview.Request) (*trpcunsafereview.Decision, error) {
	if req == nil {
		return &trpcunsafereview.Decision{Blocked: false}, nil
	}
	input := buildUnsafeTranscriptInput(req.LastUserInput, req.Transcript)
	if cat, reason := detectUnsafeIntent(input); cat != "" {
		return &trpcunsafereview.Decision{
			Blocked:  true,
			Category: cat,
			Reason:   reason,
		}, nil
	}
	return &trpcunsafereview.Decision{Blocked: false}, nil
}

func detectUnsafeIntent(input string) (trpcunsafereview.Category, string) {
	normalized := normalizeInput(input)
	physicalHarm := []string{
		"how to make a bomb",
		"how to build a weapon",
		"kill someone",
		"how to harm",
		"how to murder",
		"how to poison",
		"how to create poison",
		"how to hurt someone",
	}
	for _, p := range physicalHarm {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategoryPhysicalHarm, "potential physical harm intent detected"
		}
	}

	selfHarm := []string{
		"how to commit suicide",
		"how to self-harm",
		"end my life",
		"kill myself",
		"want to die",
		"suicide method",
	}
	for _, p := range selfHarm {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategorySelfHarm, "potential self-harm intent detected"
		}
	}

	cyberAbuse := []string{
		"how to hack",
		"exploit vulnerability",
		"create malware",
		"create ransomware",
		"ddos attack",
		"sql injection attack",
		"how to crack",
		"brute force password",
		"steal credentials",
		"gain unauthorized access",
	}
	for _, p := range cyberAbuse {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategoryCyberAbuse, "potential cyber abuse intent detected"
		}
	}

	credentialTheft := []string{
		"steal password",
		"harvest credentials",
		"keylogger",
		"phishing template",
		"credential stuffing",
	}
	for _, p := range credentialTheft {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategoryCredentialTheft, "potential credential theft intent detected"
		}
	}

	fraud := []string{
		"how to scam",
		"how to defraud",
		"create phishing",
		"steal identity",
		"counterfeit money",
		"money laundering",
		"tax evasion how",
	}
	for _, p := range fraud {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategoryFraudDeception, "potential fraud/deception intent detected"
		}
	}

	privacyAbuse := []string{
		"stalk someone online",
		"track someone without",
		"surveillance without consent",
		"dox someone",
	}
	for _, p := range privacyAbuse {
		if strings.Contains(normalized, p) {
			return trpcunsafereview.CategoryPrivacyAbuse, "potential privacy abuse intent detected"
		}
	}

	return "", ""
}

type chainedPromptInjectionReviewer struct {
	rule  trpcpromptreview.Reviewer
	deep  trpcpromptreview.Reviewer
}

var _ trpcpromptreview.Reviewer = (*chainedPromptInjectionReviewer)(nil)

func (c *chainedPromptInjectionReviewer) Review(ctx context.Context, req *trpcpromptreview.Request) (*trpcpromptreview.Decision, error) {
	dec, err := c.rule.Review(ctx, req)
	if err != nil {
		return dec, err
	}
	if dec != nil && dec.Blocked {
		return dec, nil
	}
	if c.deep != nil {
		return c.deep.Review(ctx, req)
	}
	return dec, nil
}

type chainedUnsafeIntentReviewer struct {
	rule  trpcunsafereview.Reviewer
	deep  trpcunsafereview.Reviewer
}

var _ trpcunsafereview.Reviewer = (*chainedUnsafeIntentReviewer)(nil)

func (c *chainedUnsafeIntentReviewer) Review(ctx context.Context, req *trpcunsafereview.Request) (*trpcunsafereview.Decision, error) {
	dec, err := c.rule.Review(ctx, req)
	if err != nil {
		return dec, err
	}
	if dec != nil && dec.Blocked {
		return dec, nil
	}
	if c.deep != nil {
		return c.deep.Review(ctx, req)
	}
	return dec, nil
}

type GuardrailReviewers struct {
	PromptInjectionDeep trpcpromptreview.Reviewer
	UnsafeIntentDeep    trpcunsafereview.Reviewer
}

func BuildGuardrailPluginWithReviewers(reviewers *GuardrailReviewers) (*trpcguardrail.Plugin, error) {
	piReviewer := trpcpromptreview.Reviewer(&ruleBasedPromptInjectionReviewer{})
	if reviewers != nil && reviewers.PromptInjectionDeep != nil {
		piReviewer = &chainedPromptInjectionReviewer{
			rule: &ruleBasedPromptInjectionReviewer{},
			deep: reviewers.PromptInjectionDeep,
		}
	}

	uiReviewer := trpcunsafereview.Reviewer(&ruleBasedUnsafeIntentReviewer{})
	if reviewers != nil && reviewers.UnsafeIntentDeep != nil {
		uiReviewer = &chainedUnsafeIntentReviewer{
			rule: &ruleBasedUnsafeIntentReviewer{},
			deep: reviewers.UnsafeIntentDeep,
		}
	}

	piPlugin, err := trpcpromptinjection.New(
		trpcpromptinjection.WithReviewer(piReviewer),
	)
	if err != nil {
		return nil, err
	}
	uiPlugin, err := trpcunsafeintent.New(
		trpcunsafeintent.WithReviewer(uiReviewer),
	)
	if err != nil {
		return nil, err
	}
	return trpcguardrail.New(
		trpcguardrail.WithPromptInjection(piPlugin),
		trpcguardrail.WithUnsafeIntent(uiPlugin),
	)
}

func BuildGuardrailPlugin() (*trpcguardrail.Plugin, error) {
	return BuildGuardrailPluginWithReviewers(nil)
}
