package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRootPath(t *testing.T) {
	dir := t.TempDir()

	t.Run("有效目录", func(t *testing.T) {
		got, err := NormalizeRootPath(dir)
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(got))
		assert.Equal(t, filepath.Clean(dir), got)
	})

	t.Run("尾部斜杠归一", func(t *testing.T) {
		got, err := NormalizeRootPath(dir + string(os.PathSeparator))
		require.NoError(t, err)
		assert.Equal(t, filepath.Clean(dir), got)
	})

	t.Run("空路径", func(t *testing.T) {
		_, err := NormalizeRootPath("   ")
		require.Error(t, err)
	})

	t.Run("不存在", func(t *testing.T) {
		_, err := NormalizeRootPath(filepath.Join(dir, "no-such-subdir"))
		require.Error(t, err)
	})

	t.Run("是文件而非目录", func(t *testing.T) {
		f := filepath.Join(dir, "a.md")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
		_, err := NormalizeRootPath(f)
		require.Error(t, err)
	})

	t.Run("系统根目录禁止挂载", func(t *testing.T) {
		var root string
		if runtime.GOOS == "windows" {
			root = `C:\`
		} else {
			root = "/"
		}
		_, err := NormalizeRootPath(root)
		require.Error(t, err, "系统根目录不可作为 vault root")
	})
}

func TestCreateVault(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	newUsecaseWithCapture := func(captured *Collection) *Usecase {
		return NewUsecaseFromRepo(&mockRepo{
			collCreateFn: func(_ context.Context, c Collection) (Collection, error) {
				*captured = c
				return c, nil
			},
		})
	}

	t.Run("成功：root_path 规范化 + sync_state active", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		got, err := u.CreateVault(ctx, Collection{
			Name:     "公司知识库",
			RootPath: dir + string(os.PathSeparator),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, got.ID)
		assert.Equal(t, filepath.Clean(dir), captured.RootPath)
		assert.Equal(t, "active", captured.SyncState)
		assert.Equal(t, "active", captured.Status)
	})

	t.Run("embedding 为空允许（V2 可选）", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "无向量库", RootPath: dir})
		require.NoError(t, err)
		assert.Empty(t, captured.EmbeddingModel)
	})

	t.Run("名称为空报错", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{RootPath: dir})
		require.ErrorIs(t, err, ErrNameRequired)
	})

	t.Run("root_path 非法报错且不落库", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", RootPath: filepath.Join(dir, "nope")})
		require.Error(t, err)
		assert.Empty(t, captured.ID, "repo 不应被调用")
	})

	t.Run("root_path 为空报错", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x"})
		require.Error(t, err)
	})

	t.Run("workspace 透传", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", RootPath: dir, Workspace: "ws1"})
		require.NoError(t, err)
		assert.Equal(t, "ws1", captured.Workspace)
	})
}

func TestCreateVaultRejectsSystemRoot(t *testing.T) {
	u := NewUsecaseFromRepo(&mockRepo{
		collCreateFn: func(_ context.Context, c Collection) (Collection, error) { return c, nil },
	})
	var root string
	if runtime.GOOS == "windows" {
		root = `C:\`
	} else {
		root = "/"
	}
	_, err := u.CreateVault(context.Background(), Collection{Name: "x", RootPath: root})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "root") || strings.Contains(err.Error(), "vault"))
}

// SP1-F：vault_backend 维度——local 必填 root_path / team 必须为空（设计 S6）。
func TestCreateVaultBackend(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	newUsecaseWithCapture := func(captured *Collection) *Usecase {
		return NewUsecaseFromRepo(&mockRepo{
			collCreateFn: func(_ context.Context, c Collection) (Collection, error) {
				*captured = c
				return c, nil
			},
		})
	}

	t.Run("backend 缺省归一为 local", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", RootPath: dir})
		require.NoError(t, err)
		assert.Equal(t, VaultBackendLocal, captured.VaultBackend)
	})

	t.Run("team：root_path 为空成功且不做路径规范化", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		got, err := u.CreateVault(ctx, Collection{Name: "团队库", VaultBackend: VaultBackendTeam})
		require.NoError(t, err)
		assert.NotEmpty(t, got.ID)
		assert.Equal(t, VaultBackendTeam, captured.VaultBackend)
		assert.Empty(t, captured.RootPath)
	})

	t.Run("team：设置 root_path 报错且不落库", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", VaultBackend: VaultBackendTeam, RootPath: dir})
		require.ErrorIs(t, err, ErrTeamRootPathForbidden)
		assert.Empty(t, captured.ID, "repo 不应被调用")
	})

	t.Run("local 显式：root_path 为空仍报错", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", VaultBackend: VaultBackendLocal})
		require.ErrorIs(t, err, ErrRootPathRequired)
	})

	t.Run("未知 backend 报错且不落库", func(t *testing.T) {
		var captured Collection
		u := newUsecaseWithCapture(&captured)
		_, err := u.CreateVault(ctx, Collection{Name: "x", VaultBackend: "s3", RootPath: dir})
		require.ErrorIs(t, err, ErrInvalidVaultBackend)
		assert.Empty(t, captured.ID, "repo 不应被调用")
	})
}
