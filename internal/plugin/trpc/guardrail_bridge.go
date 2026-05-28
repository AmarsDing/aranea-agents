package plugintrpc

import (
	"context"
	"strings"

	trpcguardrail "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail"
	trpcpromptinjection "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/promptinjection"
	trpcpromptreview "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/promptinjection/review"
	trpcunsafeintent "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/unsafeintent"
	trpcunsafereview "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail/unsafeintent/review"
)

type ruleBasedPromptInjectionReviewer struct{}

var _ trpcpromptreview.Reviewer = (*ruleBasedPromptInjectionReviewer)(nil)

func (r *ruleBasedPromptInjectionReviewer) Review(_ context.Context, req *trpcpromptreview.Request) (*trpcpromptreview.Decision, error) {
	if req == nil {
		return &trpcpromptreview.Decision{Blocked: false}, nil
	}
	input := strings.ToLower(req.LastUserInput)
	for _, t := range req.Transcript {
		input += " " + strings.ToLower(t.Content)
	}
	if detectPromptInjection(input) {
		return &trpcpromptreview.Decision{
			Blocked:  true,
			Category: trpcpromptreview.CategorySystemOverride,
			Reason:   "potential prompt injection pattern detected",
		}, nil
	}
	return &trpcpromptreview.Decision{Blocked: false}, nil
}

func detectPromptInjection(input string) bool {
	patterns := []string{
		"ignore previous instructions",
		"ignore all previous",
		"disregard your instructions",
		"forget your instructions",
		"you are now",
		"new instructions:",
		"system prompt:",
		"override system",
		"jailbreak",
		"pretend you are",
		"act as if you are",
		"simulate being",
		"bypass your safety",
		"bypass your rules",
		"ignore your rules",
		"ignore your guidelines",
		"do not follow your",
		"do not abide by",
		"above instructions are",
		"real instructions are",
	}
	for _, p := range patterns {
		if strings.Contains(input, p) {
			return true
		}
	}
	return false
}

type ruleBasedUnsafeIntentReviewer struct{}

var _ trpcunsafereview.Reviewer = (*ruleBasedUnsafeIntentReviewer)(nil)

func (r *ruleBasedUnsafeIntentReviewer) Review(_ context.Context, req *trpcunsafereview.Request) (*trpcunsafereview.Decision, error) {
	if req == nil {
		return &trpcunsafereview.Decision{Blocked: false}, nil
	}
	input := strings.ToLower(req.LastUserInput)
	for _, t := range req.Transcript {
		input += " " + strings.ToLower(t.Content)
	}
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
	physicalHarm := []string{"how to make a bomb", "how to build a weapon", "kill someone", "how to harm", "how to murder"}
	for _, p := range physicalHarm {
		if strings.Contains(input, p) {
			return trpcunsafereview.CategoryPhysicalHarm, "potential physical harm intent detected"
		}
	}
	cyberAbuse := []string{"how to hack", "exploit vulnerability", "create malware", "create ransomware", "ddos attack", "sql injection attack"}
	for _, p := range cyberAbuse {
		if strings.Contains(input, p) {
			return trpcunsafereview.CategoryCyberAbuse, "potential cyber abuse intent detected"
		}
	}
	fraud := []string{"how to scam", "how to defraud", "create phishing", "steal identity", "counterfeit money"}
	for _, p := range fraud {
		if strings.Contains(input, p) {
			return trpcunsafereview.CategoryFraudDeception, "potential fraud/deception intent detected"
		}
	}
	selfHarm := []string{"how to commit suicide", "how to self-harm", "end my life", "kill myself"}
	for _, p := range selfHarm {
		if strings.Contains(input, p) {
			return trpcunsafereview.CategorySelfHarm, "potential self-harm intent detected"
		}
	}
	return "", ""
}

func BuildGuardrailPlugin() (*trpcguardrail.Plugin, error) {
	piPlugin, err := trpcpromptinjection.New(
		trpcpromptinjection.WithReviewer(&ruleBasedPromptInjectionReviewer{}),
	)
	if err != nil {
		return nil, err
	}
	uiPlugin, err := trpcunsafeintent.New(
		trpcunsafeintent.WithReviewer(&ruleBasedUnsafeIntentReviewer{}),
	)
	if err != nil {
		return nil, err
	}
	return trpcguardrail.New(
		trpcguardrail.WithPromptInjection(piPlugin),
		trpcguardrail.WithUnsafeIntent(uiPlugin),
	)
}
