package sqlite

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"arenea/backend/internal/domain"
)

const avatarSize = 256

// AvatarRepository 承载头像资产与系统种子生成。
type AvatarRepository struct {
	db *sql.DB
}

// NewAvatarRepository 从 *sql.DB 构建头像仓储。
func NewAvatarRepository(db *sql.DB) *AvatarRepository {
	return &AvatarRepository{db: db}
}

type avatarSeed struct {
	id          string
	key         string
	name        string
	description string
	sortOrder   int
	bg          color.RGBA
	fg          color.RGBA
	accent      color.RGBA
}

var systemAvatarSeeds = []avatarSeed{
	{"avatar_system_01", "system-orbit-blue", "星轨蓝", "系统预置头像 1", 10, rgb(26, 115, 232), rgb(232, 240, 254), rgb(139, 180, 248)},
	{"avatar_system_02", "system-sunrise-orange", "晨光橙", "系统预置头像 2", 20, rgb(245, 124, 0), rgb(255, 247, 237), rgb(251, 191, 36)},
	{"avatar_system_03", "system-forest-green", "森林绿", "系统预置头像 3", 30, rgb(5, 150, 105), rgb(236, 253, 245), rgb(52, 211, 153)},
	{"avatar_system_04", "system-violet-core", "紫晶核", "系统预置头像 4", 40, rgb(124, 58, 237), rgb(245, 243, 255), rgb(196, 181, 253)},
	{"avatar_system_05", "system-rose-signal", "玫瑰信号", "系统预置头像 5", 50, rgb(225, 29, 72), rgb(255, 241, 242), rgb(251, 113, 133)},
	{"avatar_system_06", "system-cyan-wave", "青色波纹", "系统预置头像 6", 60, rgb(8, 145, 178), rgb(236, 254, 255), rgb(103, 232, 249)},
	{"avatar_system_07", "system-amber-bot", "琥珀机器人", "系统预置头像 7", 70, rgb(180, 83, 9), rgb(255, 251, 235), rgb(252, 211, 77)},
	{"avatar_system_08", "system-slate-node", "石板节点", "系统预置头像 8", 80, rgb(51, 65, 85), rgb(248, 250, 252), rgb(148, 163, 184)},
	{"avatar_system_09", "system-indigo-path", "靛蓝路径", "系统预置头像 9", 90, rgb(79, 70, 229), rgb(238, 242, 255), rgb(165, 180, 252)},
	{"avatar_system_10", "system-pink-spark", "粉色星火", "系统预置头像 10", 100, rgb(190, 24, 93), rgb(253, 242, 248), rgb(244, 114, 182)},
}

// SeedAvatarAssets 安装系统预置头像（迁移/启动时调用）。
func (r *AvatarRepository) SeedAvatarAssets() error {
	for _, seed := range systemAvatarSeeds {
		img, err := renderSystemAvatar(seed)
		if err != nil {
			return err
		}
		asset := domain.AvatarAsset{
			ID:            seed.id,
			Key:           seed.key,
			Name:          seed.name,
			Description:   seed.description,
			MimeType:      "image/png",
			Source:        "system",
			IsSystem:      true,
			FileSizeBytes: len(img),
			WidthPx:       avatarSize,
			HeightPx:      avatarSize,
			SortOrder:     seed.sortOrder,
		}
		if _, err = r.CreateAvatarAsset(asset, img, img); err != nil && !strings.Contains(err.Error(), "constraint failed") {
			return err
		}
	}
	return nil
}

func (r *AvatarRepository) ListAvatarAssets(scope string, workspaceID string, ownerUserID string) ([]domain.AvatarAsset, error) {
	where := []string{"deleted_at = ''", "enabled = 1", "length(image_data) > 0"}
	args := []any{}
	switch scope {
	case "system":
		where = append(where, "is_system = 1")
	case "mine":
		where = append(where, "is_system = 0")
		if workspaceID != "" {
			where = append(where, "workspace_id = ?")
			args = append(args, workspaceID)
		}
		if ownerUserID != "" {
			where = append(where, "owner_user_id = ?")
			args = append(args, ownerUserID)
		}
	default:
		// 选择器默认：包含系统资源与当前工作区上传。
		where = append(where, "(is_system = 1 OR workspace_id = ? OR owner_user_id = ?)")
		args = append(args, workspaceID, ownerUserID)
	}
	query := `SELECT id, asset_key, name, description, mime_type, workspace_id, owner_user_id, source, is_system, file_size_bytes, width_px, height_px, sort_order, created_at
		FROM avatar_assets WHERE ` + strings.Join(where, " AND ") + ` ORDER BY is_system DESC, sort_order ASC, created_at DESC`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AvatarAsset
	for rows.Next() {
		v, scanErr := scanAvatarAsset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *AvatarRepository) GetAvatarImage(id string, thumbnail bool) (domain.AvatarImage, error) {
	if id == "" {
		return domain.AvatarImage{}, errors.New("avatar id is required")
	}
	column := "image_data"
	if thumbnail {
		column = "COALESCE(NULLIF(thumbnail_data, X''), image_data)"
	}
	query := fmt.Sprintf(`SELECT id, mime_type, %s FROM avatar_assets WHERE id = ? AND deleted_at = '' AND enabled = 1`, column)
	var result domain.AvatarImage
	if err := r.db.QueryRow(query, id).Scan(&result.ID, &result.MimeType, &result.Data); err != nil {
		return domain.AvatarImage{}, err
	}
	if len(result.Data) == 0 {
		return domain.AvatarImage{}, sql.ErrNoRows
	}
	return result, nil
}

func (r *AvatarRepository) CreateAvatarAsset(asset domain.AvatarAsset, imageData []byte, thumbnailData []byte) (domain.AvatarAsset, error) {
	if asset.ID == "" || asset.Key == "" || asset.Name == "" {
		return domain.AvatarAsset{}, errors.New("id, key and name are required")
	}
	if len(imageData) == 0 {
		return domain.AvatarAsset{}, errors.New("image data is required")
	}
	if asset.MimeType == "" {
		asset.MimeType = "image/png"
	}
	if asset.Source == "" {
		asset.Source = "upload"
	}
	if asset.WidthPx == 0 {
		asset.WidthPx = avatarSize
	}
	if asset.HeightPx == 0 {
		asset.HeightPx = avatarSize
	}
	if asset.FileSizeBytes == 0 {
		asset.FileSizeBytes = len(imageData)
	}
	now := nowISO()
	asset.CreatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO avatar_assets(id, asset_key, name, image_data, thumbnail_data, mime_type, workspace_id, owner_user_id, source, is_system, file_size_bytes, width_px, height_px, description, status, enabled, sort_order, config_json, metadata_json, created_at, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 1, ?, '', '', ?, ?, '')`,
		asset.ID, asset.Key, asset.Name, imageData, thumbnailData, asset.MimeType, asset.WorkspaceID, asset.OwnerUserID, asset.Source, asset.IsSystem, asset.FileSizeBytes, asset.WidthPx, asset.HeightPx, asset.Description, asset.SortOrder, now, now,
	)
	return asset, err
}

func scanAvatarAsset(row rowScanner) (domain.AvatarAsset, error) {
	var v domain.AvatarAsset
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.MimeType, &v.WorkspaceID, &v.OwnerUserID, &v.Source, &v.IsSystem, &v.FileSizeBytes, &v.WidthPx, &v.HeightPx, &v.SortOrder, &v.CreatedAt)
	return v, err
}

func renderSystemAvatar(seed avatarSeed) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	for y := 0; y < avatarSize; y++ {
		for x := 0; x < avatarSize; x++ {
			t := float64(x+y) / float64(avatarSize*2)
			img.SetRGBA(x, y, mix(seed.bg, seed.accent, t*0.38))
		}
	}
	drawCircle(img, 128, 128, 82, withAlpha(seed.fg, 235))
	drawCircle(img, 86, 90, 20, withAlpha(seed.accent, 210))
	drawCircle(img, 170, 90, 20, withAlpha(seed.accent, 210))
	drawRoundRect(img, 74, 108, 182, 178, 24, withAlpha(seed.bg, 245))
	drawCircle(img, 105, 138, 10, seed.fg)
	drawCircle(img, 151, 138, 10, seed.fg)
	drawRoundRect(img, 104, 160, 152, 168, 4, withAlpha(seed.fg, 230))
	drawCircle(img, 202, 188, 22, withAlpha(seed.fg, 220))
	drawCircle(img, 54, 196, 14, withAlpha(seed.accent, 210))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawCircle(img *image.RGBA, cx int, cy int, radius int, c color.RGBA) {
	r2 := radius * radius
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if x < 0 || y < 0 || x >= avatarSize || y >= avatarSize {
				continue
			}
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r2 {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawRoundRect(img *image.RGBA, x1 int, y1 int, x2 int, y2 int, radius int, c color.RGBA) {
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			dx := math.Max(float64(x1+radius-x), float64(x-(x2-radius)))
			dy := math.Max(float64(y1+radius-y), float64(y-(y2-radius)))
			if dx <= 0 || dy <= 0 || dx*dx+dy*dy <= float64(radius*radius) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func rgb(r uint8, g uint8, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func withAlpha(c color.RGBA, alpha uint8) color.RGBA {
	c.A = alpha
	return c
}

func mix(a color.RGBA, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}
