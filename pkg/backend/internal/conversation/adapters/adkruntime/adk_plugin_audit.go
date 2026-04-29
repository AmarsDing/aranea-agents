package adkruntime

import (
	"log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func newRuntimeAuditPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "runtime_audit",
		OnUserMessageCallback: func(_ agent.InvocationContext, content *genai.Content) (*genai.Content, error) {
			log.Printf("adk plugin runtime_audit callback=on_user_message input=%q", previewForPlugin(contentText(content), 500))
			return nil, nil
		},
		BeforeModelCallback: func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			log.Printf("adk plugin runtime_audit callback=before_model model=%q messages=%d input=%q", req.Model, len(req.Contents), previewForPlugin(llmRequestPreview(req), 500))
			return nil, nil
		},
		AfterModelCallback: func(_ agent.CallbackContext, resp *model.LLMResponse, responseErr error) (*model.LLMResponse, error) {
			status := "success"
			if responseErr != nil {
				status = "error"
			}
			log.Printf("adk plugin runtime_audit callback=after_model status=%s output=%q error=%v", status, previewForPlugin(llmResponseText(resp), 500), responseErr)
			return nil, nil
		},
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			log.Printf("adk plugin runtime_audit callback=before_tool tool=%q args=%q", t.Name(), previewForPlugin(redactText(mustJSON(args)), 500))
			return nil, nil
		},
		AfterToolCallback: func(_ tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
			status := "success"
			if err != nil {
				status = "error"
			}
			log.Printf("adk plugin runtime_audit callback=after_tool tool=%q status=%s result=%q error=%v", t.Name(), status, previewForPlugin(redactText(mustJSON(result)), 500), err)
			return nil, nil
		},
		OnToolErrorCallback: func(_ tool.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
			log.Printf("adk plugin runtime_audit callback=on_tool_error tool=%q args=%q error=%v", t.Name(), previewForPlugin(redactText(mustJSON(args)), 500), err)
			return nil, nil
		},
		OnEventCallback: func(_ agent.InvocationContext, event *session.Event) (*session.Event, error) {
			if event != nil {
				log.Printf("adk plugin runtime_audit callback=on_event author=%q final=%t output=%q", event.Author, event.IsFinalResponse(), previewForPlugin(llmResponseText(&event.LLMResponse), 500))
			}
			return event, nil
		},
	})
}
