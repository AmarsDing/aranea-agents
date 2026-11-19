package data

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/auth"
)

// TestParseTokenVersionBumpPayload 覆盖 P1-4 事件载荷解析（Bump 发布、
// SubscribeSessionRevocations 消费，两端共用格式 "<userID>:<newVersion>"）。
func TestParseTokenVersionBumpPayload(t *testing.T) {
	cases := []struct {
		payload     string
		wantUser    int64
		wantVersion int64
		wantErr     bool
	}{
		{payload: "7:3", wantUser: 7, wantVersion: 3},
		{payload: "9007199254740993:1", wantUser: 9007199254740993, wantVersion: 1},
		{payload: "", wantErr: true},
		{payload: "7", wantErr: true},
		{payload: ":3", wantErr: true},
		{payload: "7:", wantErr: true},
		{payload: "x:3", wantErr: true},
		{payload: "7:y", wantErr: true},
	}
	for _, tc := range cases {
		userID, version, err := parseTokenVersionBumpPayload(tc.payload)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("payload %q: expected error, got (%d,%d)", tc.payload, userID, version)
			}
			continue
		}
		if err != nil {
			t.Fatalf("payload %q: unexpected error %v", tc.payload, err)
		}
		if userID != tc.wantUser || version != tc.wantVersion {
			t.Fatalf("payload %q: got (%d,%d), want (%d,%d)", tc.payload, userID, version, tc.wantUser, tc.wantVersion)
		}
	}
}

// TestSubscribeSessionRevocationsWithoutRedis 未配置 Redis 时订阅端口返回
// 永久不可用哨兵（P1-4 R11：调用方据此停止重试，改由兜底扫描收敛）。
func TestSubscribeSessionRevocationsWithoutRedis(t *testing.T) {
	store := NewTokenVersionStore(nil, nil, nil)
	err := store.SubscribeSessionRevocations(context.Background(), func(int64, int64) {})
	if !errors.Is(err, auth.ErrRevocationSubscriptionUnavailable) {
		t.Fatalf("err = %v, want ErrRevocationSubscriptionUnavailable", err)
	}
}
