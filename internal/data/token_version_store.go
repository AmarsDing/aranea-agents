package data

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	entadmin "aranea-agents/internal/data/ent/admin"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// 本文件实现三项目整合 P1-3 会话吊销存储（P0 详设 01 §5）：
// token_version 的 Redis write-through 缓存 + DB 权威源。
//
// 一致性设计（缓存族为 write-through + cache-aside 混合）：
//   - Bump：先 DB 递增（权威），后 Redis 强制 SET 新版本（write-through）；
//     Redis 写失败降级 DEL，后续读回源 DB，绝不靠 TTL 收敛。
//   - CurrentVersion：Redis 命中即用；miss/出错回源 DB，回填用 SET NX——
//     与 Bump 的强制 SET 配合，保证并发下缓存最终收敛到 DB 最新版本
//     （bump 的强制 SET 是 last-writer，回填 NX 不会覆盖它）。
//   - 用户删除（not found）：返回错误，中间件 fail-closed，被删用户 token 立即失效。

const (
	// tokenVersionCacheKeyPrefix 是 token_version 缓存键前缀（auth:tv:<userID>）。
	tokenVersionCacheKeyPrefix = "auth:tv:"
	// TokenVersionBumpChannel 是 tv 递增事件的 Redis pub/sub 频道
	// （payload "<userID>:<newVersion>"；P1-4 WS 主动关闭消费）。
	TokenVersionBumpChannel = "auth:tv_bump"
)

// TokenVersionStore implements auth.TokenVersionChecker（Redis write-through +
// DB 回源），并暴露 Bump 作为全部吊销触发点（踢会话/改密/离职/角色密级变更）
// 的唯一入口。
type TokenVersionStore struct {
	d   *Data
	rdb *RedisClient
	lg  loggateway.Logger
}

var (
	_ auth.TokenVersionChecker = (*TokenVersionStore)(nil)
	_ biz.SessionRevoker       = (*TokenVersionStore)(nil)
)

// NewTokenVersionStore constructs the store. rdb 为 nil（未配置/不可用）时
// 退化为纯 DB 直查——功能仍正确，仅失去缓存与跨进程 pub/sub（单进程部署可接受；
// 多进程部署必须配置 Redis，否则 WS 主动关闭事件丢失，见 P1-4 R11）。
func NewTokenVersionStore(d *Data, rdb *RedisClient, lg loggateway.Logger) *TokenVersionStore {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TokenVersionStore{d: d, rdb: rdb, lg: lg}
}

// CurrentVersion returns the current admins.token_version for userID.
func (s *TokenVersionStore) CurrentVersion(ctx context.Context, userID int64) (int64, error) {
	if s.rdb != nil && s.rdb.IsEnabled() {
		val, err := s.rdb.Get(ctx, tokenVersionCacheKeyPrefix+strconv.FormatInt(userID, 10)).Result()
		if err == nil {
			if v, perr := strconv.ParseInt(val, 10, 64); perr == nil {
				return v, nil
			}
			// 脏缓存：删除后回源 DB。
			_ = s.rdb.Del(ctx, tokenVersionCacheKeyPrefix+strconv.FormatInt(userID, 10)).Err()
		}
	}
	v, err := s.currentVersionFromDB(ctx, userID)
	if err != nil {
		return 0, err
	}
	if s.rdb != nil && s.rdb.IsEnabled() {
		// SET NX：不覆盖并发 Bump 的强制 SET（last-writer 收敛保证）。
		if err := s.rdb.SetNX(ctx, tokenVersionCacheKeyPrefix+strconv.FormatInt(userID, 10), v, 0).Err(); err != nil {
			s.lg.Warn("token version cache backfill failed",
				loggateway.StepID("auth.tv_backfill"), loggateway.Int64("user_id", userID), loggateway.Err(err))
		}
	}
	return v, nil
}

// Bump 原子递增 userID 的 token_version，使该用户全部已签发会话立即失效；
// 成功后将新版本写入缓存并向 TokenVersionBumpChannel 广播（P1-4 WS 消费）。
// 返回递增后的新版本。
func (s *TokenVersionStore) Bump(ctx context.Context, userID int64) (int64, error) {
	po, err := s.d.RW().Write(ctx).Admin.UpdateOneID(userID).
		AddTokenVersion(1).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, biz.ErrAdminNotFound
		}
		return 0, entErrToBizErr(err, "ADMIN")
	}
	newVersion := po.TokenVersion
	key := tokenVersionCacheKeyPrefix + strconv.FormatInt(userID, 10)
	if s.rdb != nil && s.rdb.IsEnabled() {
		// 强制 SET（write-through last-writer）；失败降级 DEL，读侧回源 DB。
		if err := s.rdb.Set(ctx, key, newVersion, 0).Err(); err != nil {
			s.lg.Warn("token version cache write-through failed; invalidating",
				loggateway.StepID("auth.tv_bump"), loggateway.Int64("user_id", userID), loggateway.Err(err))
			_ = s.rdb.Del(ctx, key).Err()
		}
		// pub/sub 广播为尽力而为：订阅方（P1-4 WS）丢消息时，重连握手仍被
		// tv 校验拒绝；WS 侧 fail-closed 策略由 P1-4 落实（R11）。
		payload := fmt.Sprintf("%d:%d", userID, newVersion)
		if err := s.rdb.Publish(ctx, TokenVersionBumpChannel, payload).Err(); err != nil {
			s.lg.Warn("token version bump publish failed",
				loggateway.StepID("auth.tv_bump"), loggateway.Int64("user_id", userID), loggateway.Err(err))
		}
	}
	s.lg.Info("token version bumped; all sessions revoked",
		loggateway.StepID("auth.tv_bump"), loggateway.Int64("user_id", userID), loggateway.Int64("token_version", newVersion))
	return newVersion, nil
}

// RevokeSessions implements biz.SessionRevoker（biz 端口与数据层 Bump 同义，
// 分名仅为层间语义）。
func (s *TokenVersionStore) RevokeSessions(ctx context.Context, userID int64) (int64, error) {
	return s.Bump(ctx, userID)
}

// SubscribeSessionRevocations 阻塞订阅 TokenVersionBumpChannel，收到事件即回调
// onBump（P1-4：WS 长连接主动关闭）。回调在订阅 goroutine 上串行执行，实现方
// 必须快速返回（关闭连接是本地操作）。
//
// 返回：
//   - ctx 取消 / 频道关闭：返回相应错误，调用方可退避重连；
//   - Redis 未配置或未启用：返回 auth.ErrRevocationSubscriptionUnavailable，
//     调用方应停止重试（该部署没有跨进程事件通道）。
func (s *TokenVersionStore) SubscribeSessionRevocations(ctx context.Context, onBump func(userID int64, newVersion int64)) error {
	if s.rdb == nil || !s.rdb.IsEnabled() {
		return auth.ErrRevocationSubscriptionUnavailable
	}
	sub := s.rdb.Subscribe(ctx, TokenVersionBumpChannel)
	defer func() { _ = sub.Close() }()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return errors.New("auth: token version bump channel closed")
			}
			userID, version, err := parseTokenVersionBumpPayload(msg.Payload)
			if err != nil {
				s.lg.Warn("token version bump payload invalid",
					loggateway.StepID("auth.tv_subscribe"), loggateway.Err(err))
				continue
			}
			onBump(userID, version)
		}
	}
}

// parseTokenVersionBumpPayload 解析 "<userID>:<newVersion>"。
func parseTokenVersionBumpPayload(payload string) (int64, int64, error) {
	sep := strings.IndexByte(payload, ':')
	if sep <= 0 || sep == len(payload)-1 {
		return 0, 0, fmt.Errorf("malformed token version bump payload %q", payload)
	}
	userID, err := strconv.ParseInt(payload[:sep], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed user id in payload %q: %w", payload, err)
	}
	version, err := strconv.ParseInt(payload[sep+1:], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed version in payload %q: %w", payload, err)
	}
	return userID, version, nil
}

func (s *TokenVersionStore) currentVersionFromDB(ctx context.Context, userID int64) (int64, error) {
	po, err := s.d.RW().Read(ctx).Admin.Query().
		Select(entadmin.FieldID, entadmin.FieldTokenVersion).
		Where(entadmin.IDEQ(userID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, biz.ErrAdminNotFound
		}
		return 0, entErrToBizErr(err, "ADMIN")
	}
	return po.TokenVersion, nil
}
