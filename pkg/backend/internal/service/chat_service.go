// 薄装配层：自进化/路由/回合执行实现位于
// arenea/backend/internal/conversation/application（0 main design §12.1.1 行 #22、#24）。
// 新代码请直接 import conversation/application；P8 将移除此包装。
package service

import (
	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/conversation/application"
	"arenea/backend/internal/repository"
)

// ChatService 由 conversation 用例与团队编排组成；*application.ChatService 上挂全部方法，此处仅嵌入以保留
// internal/service 路径下的类型名，供 transport / CLI 少改 import。
type ChatService struct {
	*application.ChatService
}

// NewChatService 装配 ChatService（L0–L4、自进化、adkruntime、团队事件总线等）。
func NewChatService(repo repository.Store, runtimeAdapter *adkr.ADKRuntimeAdapter) *ChatService {
	return &ChatService{application.NewChatService(repo, runtimeAdapter)}
}

type (
	AttachmentRefInput  = application.AttachmentRefInput
	SendMessageInput    = application.SendMessageInput
	SendMessageOptions  = application.SendMessageOptions
	SendMessageResult   = application.SendMessageResult
	SendStreamCallbacks = application.SendStreamCallbacks
	TeamRunEvent        = application.TeamRunEvent
	TeamRunEventBroker  = application.TeamRunEventBroker
)

// NewTeamRunEventBroker 转发至 conversation application（团队 SSE 用）。
var NewTeamRunEventBroker = application.NewTeamRunEventBroker
