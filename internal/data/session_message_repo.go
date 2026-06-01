package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/message"
	entsession "aranea-agents/internal/data/ent/session"
	entsessionturn "aranea-agents/internal/data/ent/sessionturn"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func entMessageToBiz(m *ent.Message) biz.ChatMessage {
	if m == nil {
		return biz.ChatMessage{}
	}
	return biz.ChatMessage{
		ID: m.ID, SessionID: m.SessionID, ParentMessageID: m.ParentMessageID,
		TurnID: m.TurnID, TurnNumber: m.TurnNumber, SeqInTurn: m.SeqInTurn, Role: m.Role, ContentMarkdown: m.ContentMarkdown,
		ModelName: m.ModelName, TokenIn: m.TokenIn, TokenOut: m.TokenOut,
		LatencyMS: m.LatencyMs, Status: m.Status, AttachmentsCount: m.AttachmentsCount,
		OptionsJSON: m.OptionsJSON, ErrorMessage: m.ErrorMessage, CreatedAt: m.CreatedAt,
	}
}

func (r *sessionRepo) CountMessagesBySession(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	return r.txClient(ctx).Message.Query().Where(message.SessionIDEQ(sessionID)).Count(ctx)
}

func (r *sessionRepo) ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	if limit <= 0 {
		limit = biz.MessageListDefaultLimit
	}
	if limit > biz.MessageListMaxLimit {
		limit = biz.MessageListMaxLimit
	}
	rows, err := r.txClient(ctx).Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnID(entsql.OrderAsc()), message.BySeqInTurn(entsql.OrderAsc()), message.ByCreatedAt(entsql.OrderAsc())).
		Limit(limit).Offset(clampOffset(offset)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	turnIDs, err := r.txClient(ctx).SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID), entsessionturn.TurnNumberGT(afterTurn)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(turnIDs))
	for _, t := range turnIDs {
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return []biz.ChatMessage{}, nil
	}
	q := r.txClient(ctx).Message.Query().Where(message.SessionIDEQ(sessionID), message.TurnIDIn(ids...))
	rows, err := q.Order(message.ByTurnID(entsql.OrderAsc()), message.BySeqInTurn(entsql.OrderAsc()), message.ByCreatedAt(entsql.OrderAsc())).
		Limit(biz.CompressMessageMaxRows).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]biz.ChatMessage, error) {
	sessionID, status = strings.TrimSpace(sessionID), strings.TrimSpace(status)
	if sessionID == "" || status == "" {
		return nil, kerrors.BadRequest("SESSION", "session id and status are required")
	}
	if limit <= 0 || limit > biz.ActivityCancelScanLimit {
		limit = biz.ActivityCancelScanLimit
	}
	rows, err := r.txClient(ctx).Message.Query().
		Where(message.SessionIDEQ(sessionID), message.StatusEQ(status)).
		Order(message.ByTurnID(entsql.OrderDesc()), message.BySeqInTurn(entsql.OrderDesc()), message.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, entMessageToBiz(m))
	}
	return out, nil
}

func (r *sessionRepo) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	if limit <= 0 || limit > biz.TimelineMessageMaxFetch {
		limit = biz.TimelineMessageMaxFetch
	}
	rows, err := r.txClient(ctx).Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(message.ByTurnID(entsql.OrderDesc()), message.BySeqInTurn(entsql.OrderDesc()), message.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChatMessage, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		out = append(out, entMessageToBiz(rows[i]))
	}
	return out, nil
}

func (r *sessionRepo) maxMessageTurnTx(ctx context.Context, tx *ent.Tx, sessionID string) (int, error) {
	row, err := tx.SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID)).
		Order(entsessionturn.ByTurnNumber(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return row.TurnNumber, nil
}

// assignTurnForNewMessage determines whether a new message reuses the latest
// turn or creates a new one.
//
// State transition:
//
//	latest turn status      �?message role �?action
//	────────────────────────┼──────────────┼──────────────────────────
//	awaiting_user           �?user         �?reuse (fill user slot)
//	awaiting_user           �?non-user     �?reuse (append to active turn)
//	running / completing    �?non-user     �?reuse (append to active turn)
//	running / completing    �?user         �?new turn (status=running)
//	completed / failed /    �?any          �?new turn (status=running)
//	cancelled               �?             �?//	no existing turn        �?any          �?new turn (status=running)
func (r *sessionRepo) assignTurnForNewMessage(ctx context.Context, tx *ent.Tx, sessionID, role string) (turnID string, turnNumber int, seqInTurn int, err error) {
	latestTurn, qErr := tx.SessionTurn.Query().
		Where(entsessionturn.SessionIDEQ(sessionID)).
		Order(entsessionturn.ByTurnNumber(entsql.OrderDesc())).
		First(ctx)
	if qErr != nil && !ent.IsNotFound(qErr) {
		return "", 0, 0, qErr
	}
	if latestTurn != nil {
		shouldReuse := false
		switch role {
		case "user":
			shouldReuse = latestTurn.Status == "awaiting_user"
		default:
			shouldReuse = latestTurn.Status != "completed" && latestTurn.Status != "failed" && latestTurn.Status != "cancelled"
		}
		if shouldReuse {
			maxSeq, seqErr := tx.Message.Query().
				Where(message.SessionIDEQ(sessionID), message.TurnIDEQ(latestTurn.ID)).
				Order(message.BySeqInTurn(entsql.OrderDesc())).
				First(ctx)
			if seqErr != nil && !ent.IsNotFound(seqErr) {
				return "", 0, 0, seqErr
			}
			nextSeq := 1
			if maxSeq != nil {
				nextSeq = maxSeq.SeqInTurn + 1
			}
			return latestTurn.ID, latestTurn.TurnNumber, nextSeq, nil
		}
	}
	maxTurn, mErr := r.maxMessageTurnTx(ctx, tx, sessionID)
	if mErr != nil {
		return "", 0, 0, mErr
	}
	newTurnID := uuid.NewString()
	newTurnNumber := maxTurn + 1
	now := nowRFC3339()
	if _, cErr := tx.SessionTurn.Create().
		SetID(newTurnID).
		SetSessionID(sessionID).
		SetTurnNumber(newTurnNumber).
		SetStatus("running").
		SetStartedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); cErr != nil {
		return "", 0, 0, cErr
	}
	return newTurnID, newTurnNumber, 1, nil
}

func (r *sessionRepo) insertMessageTx(ctx context.Context, tx *ent.Tx, m biz.ChatMessage) error {
	return tx.Message.Create().
		SetID(m.ID).
		SetSessionID(m.SessionID).
		SetParentMessageID(m.ParentMessageID).
		SetTurnID(m.TurnID).
		SetTurnNumber(m.TurnNumber).
		SetSeqInTurn(m.SeqInTurn).
		SetRole(m.Role).
		SetContentMarkdown(m.ContentMarkdown).
		SetModelName(m.ModelName).
		SetTokenIn(m.TokenIn).
		SetTokenOut(m.TokenOut).
		SetLatencyMs(m.LatencyMS).
		SetStatus(m.Status).
		SetAttachmentsCount(m.AttachmentsCount).
		SetOptionsJSON(m.OptionsJSON).
		SetErrorMessage(m.ErrorMessage).
		SetCreatedAt(m.CreatedAt).
		Exec(ctx)
}

func (r *sessionRepo) AppendChatTurn(ctx context.Context, sessionID string, user, assistant biz.ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	tx, err := r.txClient(ctx).Tx(ctx)
	if err != nil {
		r.data.lg.Error("tx begin failed", loggateway.StepID("data.session.append_turn.tx_begin"), loggateway.Err(err))
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return err
	}
	maxTurn, err := r.maxMessageTurnTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	turnID := uuid.NewString()
	turnNumber := maxTurn + 1
	now := nowRFC3339()
	if _, err = tx.SessionTurn.Create().
		SetID(turnID).
		SetSessionID(sessionID).
		SetTurnNumber(turnNumber).
		SetUserMessageID(user.ID).
		SetAssistantMessageID(assistant.ID).
		SetStatus("completed").
		SetStartedAt(now).
		SetEndedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return err
	}
	user.TurnID = turnID
	user.TurnNumber = turnNumber
	user.SeqInTurn = 1
	assistant.TurnID = turnID
	assistant.TurnNumber = turnNumber
	assistant.SeqInTurn = 2
	if err = r.insertMessageTx(ctx, tx, user); err != nil {
		return err
	}
	if err = r.insertMessageTx(ctx, tx, assistant); err != nil {
		return err
	}
	upd := tx.Session.UpdateOneID(sessionID).
		AddMessageCount(2).
		SetLastMessageAt(assistant.CreatedAt).
		SetUpdatedAt(nowRFC3339()).
		AddModelCallCount(1)
	if tin, tout := assistant.TokenIn, assistant.TokenOut; tin > 0 || tout > 0 {
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout)
	}
	if _, err = upd.Save(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		r.data.lg.Error("tx commit failed", loggateway.StepID("data.session.append_turn.commit"), loggateway.Err(err))
		return err
	}
	return nil
}

// UpsertChatActivityMessage still uses manual Tx instead of ExecInTx + txClient.
// sessionRepo has no txClient helper yet; refactor when available.
func (r *sessionRepo) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg biz.ChatMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	msg.ID = strings.TrimSpace(msg.ID)
	if msg.ID == "" {
		return kerrors.BadRequest("SESSION", "message id is required")
	}
	tx, err := r.txClient(ctx).Tx(ctx)
	if err != nil {
		r.data.lg.Error("tx begin failed", loggateway.StepID("data.session.upsert_msg.tx_begin"), loggateway.Err(err))
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return err
	}
	existing, err := tx.Message.Query().Where(message.IDEQ(msg.ID), message.SessionIDEQ(sessionID)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		msg.SessionID = sessionID
		turnID, turnNum, seqInTurn, merr := r.assignTurnForNewMessage(ctx, tx, sessionID, msg.Role)
		if merr != nil {
			return merr
		}
		msg.TurnID = turnID
		msg.TurnNumber = turnNum
		msg.SeqInTurn = seqInTurn
		if strings.TrimSpace(msg.CreatedAt) == "" {
			msg.CreatedAt = nowRFC3339()
		}
		if err = r.insertMessageTx(ctx, tx, msg); err != nil {
			return err
		}
		if _, err = tx.Session.UpdateOneID(sessionID).
			AddMessageCount(1).
			SetLastMessageAt(msg.CreatedAt).
			SetUpdatedAt(nowRFC3339()).
			Save(ctx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			r.data.lg.Error("tx commit failed", loggateway.StepID("data.session.upsert_msg.commit"), loggateway.Err(err))
			return err
		}
		return nil
	}
	lastAt := msg.CreatedAt
	if strings.TrimSpace(lastAt) == "" {
		lastAt = existing.CreatedAt
	}
	update := tx.Message.UpdateOneID(msg.ID).
		SetContentMarkdown(msg.ContentMarkdown).
		SetOptionsJSON(msg.OptionsJSON).
		SetStatus(msg.Status).
		SetLatencyMs(msg.LatencyMS).
		SetErrorMessage(msg.ErrorMessage)
	if msg.TokenIn > 0 {
		update = update.SetTokenIn(msg.TokenIn)
	}
	if msg.TokenOut > 0 {
		update = update.SetTokenOut(msg.TokenOut)
	}
	if msg.ModelName != "" {
		update = update.SetModelName(msg.ModelName)
	}
	if _, err = update.Save(ctx); err != nil {
		return err
	}
	if _, err = tx.Session.UpdateOneID(sessionID).
		SetLastMessageAt(lastAt).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		r.data.lg.Error("tx commit failed", loggateway.StepID("data.session.upsert_msg.commit"), loggateway.Err(err))
		return err
	}
	return nil
}

func (r *sessionRepo) AppendChatMessage(ctx context.Context, sessionID string, msg biz.ChatMessage, bumpModelCall bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	tx, err := r.txClient(ctx).Tx(ctx)
	if err != nil {
		r.data.lg.Error("tx begin failed", loggateway.StepID("data.session.append_msg.tx_begin"), loggateway.Err(err))
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
		return err
	}
	turnID, turnNum, seqInTurn, err := r.assignTurnForNewMessage(ctx, tx, sessionID, msg.Role)
	if err != nil {
		return err
	}
	msg.TurnID = turnID
	msg.TurnNumber = turnNum
	msg.SeqInTurn = seqInTurn
	if err = r.insertMessageTx(ctx, tx, msg); err != nil {
		if ent.IsConstraintError(err) {
			return biz.ErrMessageDuplicate
		}
		return err
	}
	upd := tx.Session.UpdateOneID(sessionID).
		AddMessageCount(1).
		SetLastMessageAt(msg.CreatedAt).
		SetUpdatedAt(nowRFC3339())
	if bumpModelCall {
		upd = upd.AddModelCallCount(1)
	}
	tin, tout := msg.TokenIn, msg.TokenOut
	if bumpModelCall && (tin > 0 || tout > 0) {
		upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin + tout)
	}
	if _, err = upd.Save(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		r.data.lg.Error("tx commit failed", loggateway.StepID("data.session.append_msg.commit"), loggateway.Err(err))
		return err
	}
	return nil
}

func (r *sessionRepo) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	status = strings.TrimSpace(status)
	if sessionID == "" || messageID == "" {
		return kerrors.BadRequest("SESSION", "session_id and message_id are required")
	}
	if status == "" {
		return kerrors.BadRequest("SESSION", "status is required")
	}
	_, err := r.txClient(ctx).Message.Update().
		Where(message.IDEQ(messageID), message.SessionIDEQ(sessionID)).
		SetStatus(status).
		SetErrorMessage(strings.TrimSpace(errorMessage)).
		Save(ctx)
	return err
}

func (r *sessionRepo) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]biz.ChatMessage, error) {
	if afterRevision <= 0 {
		return r.ListMessagesBySession(ctx, sessionID, biz.MessageListMaxLimit, 0)
	}
	return r.ListMessagesAfterTurn(ctx, sessionID, int(afterRevision))
}
