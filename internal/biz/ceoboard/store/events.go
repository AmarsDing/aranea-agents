// 事件查询与 JSON 保真编解码(自 ceo-dashboard/internal/store/events.go 移植)。
// 保真要点:
//   - MarshalJSON/UnmarshalJSON:合法 UTF-8 保持原文 JSON 字符串;非法字节用 base64
//     扩展字段保真(风险点⑪);
//   - QueryEvents 所有 subject/event_id 条件均为字面相等,AllowedSubjects 白名单
//     不解释 NATS 通配符(风险点⑦服务端权限裁剪);
//   - 游标 (received_at, event_uid) 双键倒序,防同毫秒翻页漏读。
package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/ceoboardevent"
)

// MarshalJSON 对合法 UTF-8 保持原文 JSON 字符串;非法字节用 base64 扩展字段保真。
func (e Event) MarshalJSON() ([]byte, error) {
	type eventAlias Event
	if utf8.ValidString(e.Payload) {
		return json.Marshal(eventAlias(e))
	}
	return json.Marshal(struct {
		eventAlias
		Payload         string `json:"Payload"`
		PayloadEncoding string `json:"PayloadEncoding"`
	}{
		eventAlias:      eventAlias(e),
		Payload:         base64.StdEncoding.EncodeToString([]byte(e.Payload)),
		PayloadEncoding: "base64",
	})
}

// UnmarshalJSON 与 MarshalJSON 对称,识别 base64 扩展字段。
func (e *Event) UnmarshalJSON(raw []byte) error {
	type eventAlias Event
	var decoded struct {
		eventAlias
		PayloadEncoding string `json:"PayloadEncoding"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*e = Event(decoded.eventAlias)
	switch decoded.PayloadEncoding {
	case "":
		return nil
	case "base64":
		payload, err := base64.StdEncoding.DecodeString(e.Payload)
		if err != nil {
			return fmt.Errorf("解码 base64 payload: %w", err)
		}
		e.Payload = string(payload)
		return nil
	default:
		return fmt.Errorf("未知 PayloadEncoding: %s", decoded.PayloadEncoding)
	}
}

// QueryEvents 按权限白名单查询事件;所有 subject/event_id 条件均为字面相等。
func (s *Store) QueryEvents(ctx context.Context, filter EventFilter) ([]Event, error) {
	if filter.EventID != "" && filter.Platform == "" {
		return nil, fmt.Errorf("event_id 查询必须指定 platform")
	}
	if len(filter.AllowedSubjects) == 0 {
		return []Event{}, nil
	}
	allowed := make(map[string]bool, len(filter.AllowedSubjects))
	for _, subject := range filter.AllowedSubjects {
		allowed[subject] = true
	}
	if filter.Subject != "" && !allowed[filter.Subject] {
		return nil, fmt.Errorf("subject 不在权限白名单内")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := s.client.CeoboardEvent.Query().
		Where(ceoboardevent.SubjectIn(filter.AllowedSubjects...))
	if filter.Platform != "" {
		q = q.Where(ceoboardevent.PlatformEQ(filter.Platform))
	}
	if filter.Subject != "" {
		q = q.Where(ceoboardevent.SubjectEQ(filter.Subject))
	}
	if filter.EventID != "" {
		q = q.Where(ceoboardevent.EventIDEQ(filter.EventID))
	}
	if !filter.From.IsZero() {
		q = q.Where(ceoboardevent.ReceivedAtGTE(filter.From.UTC()))
	}
	if !filter.To.IsZero() {
		q = q.Where(ceoboardevent.ReceivedAtLT(filter.To.UTC()))
	}
	if filter.Cursor != nil {
		curAt := filter.Cursor.ReceivedAt.UTC()
		q = q.Where(ceoboardevent.Or(
			ceoboardevent.ReceivedAtLT(curAt),
			ceoboardevent.And(
				ceoboardevent.ReceivedAtEQ(curAt),
				ceoboardevent.IDLT(filter.Cursor.UID),
			),
		))
	}
	rows, err := q.
		Order(ent.Desc(ceoboardevent.FieldReceivedAt), ent.Desc(ceoboardevent.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, e := range rows {
		out = append(out, eventToDTO(e))
	}
	return out, nil
}

// GetEvent 返回一条事件的完整原文;调用方必须重新校验 subject 权限。
func (s *Store) GetEvent(ctx context.Context, uid string) (*Event, error) {
	e, err := s.client.CeoboardEvent.Get(ctx, uid)
	if err != nil {
		return nil, err
	}
	ev := eventToDTO(e)
	return &ev, nil
}
