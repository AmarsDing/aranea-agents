package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// 工作台 › 资料 P1 的本机动作（设计 10-资料.md §3.3、§4）：建库向导选本机文件夹、
// 「用本机程序打开」。都是对后端所在机器桌面的操作，和 /v1/system/reveal 同属
// 单机本地部署功能：路由只在 FEATURES_LOCAL_REVEAL_ENABLED 打开时注册，且只给管理员。

// errOpenUnsupported 当前系统不支持「用本机程序打开」。
var errOpenUnsupported = errors.New("open with default app is not supported on this OS")

// openWithDefaultApp 用系统默认程序打开文件（平台实现见 knowledge_open_*.go）；测试可替换。
var openWithDefaultApp = launchDefaultApp

// localOpenExts 允许「用本机程序打开」的类型：只放文档 / 表格 / 演示 / 图片 / 音视频 / 压缩包。
// 资料库里可能被上传进 exe、bat、ps1、lnk、html 之类，用默认程序「打开」它们就是执行，一律不放行。
var localOpenExts = map[string]bool{
	".doc": true, ".docx": true, ".docm": true, ".dot": true, ".dotx": true, ".rtf": true, ".odt": true, ".wps": true,
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".log": true, ".json": true, ".xml": true, ".yaml": true, ".yml": true,
	".xls": true, ".xlsx": true, ".xlsm": true, ".xlt": true, ".xltx": true, ".ods": true, ".et": true,
	".ppt": true, ".pptx": true, ".pptm": true, ".pps": true, ".ppsx": true, ".odp": true, ".dps": true,
	".pdf": true, ".ofd": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true, ".tif": true, ".tiff": true, ".heic": true, ".svg": true,
	".mp3": true, ".wav": true, ".m4a": true, ".flac": true, ".aac": true, ".ogg": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".wmv": true, ".webm": true,
	".zip": true, ".rar": true, ".7z": true,
}

// localDirLimit 一层最多列多少个子目录（防止超大目录把响应撑爆）。
const localDirLimit = 500

type localDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type localDirListing struct {
	// Path 当前目录；空 = 根层（主目录 + 各盘符）。
	Path string `json:"path"`
	// Parent 上一级；空 = 回到根层。
	Parent string          `json:"parent"`
	Dirs   []localDirEntry `json:"dirs"`
}

// requireLocalAdmin 本机动作只给管理员。
func requireLocalAdmin(w http.ResponseWriter, r *http.Request) bool {
	a, ok := auth.FromContext(r.Context())
	if !ok || a == nil {
		writeKnowledgeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	if !a.HasAdminAccess() {
		writeKnowledgeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return false
	}
	return true
}

// ServeLocalDirs GET /v1/knowledge/local-dirs?path= 列本机目录（只列文件夹，不列文件）。
func (s *KnowledgeService) ServeLocalDirs(w http.ResponseWriter, r *http.Request) {
	if !requireLocalAdmin(w, r) {
		return
	}
	listing, err := listLocalDirs(r.URL.Query().Get("path"))
	if err != nil {
		writeKnowledgeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, listing)
}

// ServeCreateLocalDir POST /v1/knowledge/local-dirs {parent, name} 在本机新建一个文件夹（建库向导用）。
func (s *KnowledgeService) ServeCreateLocalDir(w http.ResponseWriter, r *http.Request) {
	if !requireLocalAdmin(w, r) {
		return
	}
	var req struct {
		Parent string `json:"parent"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeKnowledgeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	abs, err := createLocalDir(req.Parent, req.Name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrExist) {
			status = http.StatusConflict
		}
		writeKnowledgeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	s.lg.Info("资料：本机新建文件夹", loggateway.StepID("knowledge.local_dir.create"), loggateway.Str("path", abs))
	writeKnowledgeJSON(w, http.StatusOK, map[string]string{"path": abs})
}

// ServeOpenLocal POST /v1/knowledge/documents/{id}/open-local 用本机默认程序打开资料库里的文件。
// 412 = 不是本机文件夹里的文件（团队库）；400 = 类型不放行；501 = 系统不支持。前端据此改成下载。
func (s *KnowledgeService) ServeOpenLocal(w http.ResponseWriter, r *http.Request, docID string) {
	if !requireLocalAdmin(w, r) {
		return
	}
	if _, _, err := s.requireDocumentRead(r.Context(), docID); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	ref, err := s.uc.ResolveDocumentAsset(r.Context(), docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if ref.AbsPath == "" {
		writeKnowledgeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "not a local file"})
		return
	}
	if !localOpenExts[strings.ToLower(filepath.Ext(ref.AbsPath))] {
		writeKnowledgeJSON(w, http.StatusBadRequest, map[string]string{"error": "file type not allowed"})
		return
	}
	if err := openWithDefaultApp(ref.AbsPath); err != nil {
		if errors.Is(err, errOpenUnsupported) {
			writeKnowledgeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not supported"})
			return
		}
		s.lg.Warn("资料：用本机程序打开失败", loggateway.StepID("knowledge.open_local_fail"), loggateway.Str("doc_id", docID), loggateway.Err(err))
		writeKnowledgeJSONError(w, apierror.Internal("KNOWLEDGE", "open local file failed"))
		return
	}
	s.lg.Info("资料：用本机程序打开", loggateway.StepID("knowledge.open_local"), loggateway.Str("doc_id", docID))
	writeKnowledgeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// listLocalDirs 列 dir 的直接子文件夹；dir 空 = 根层（主目录 + 各盘符）。跳过隐藏和系统目录。
func listLocalDirs(dir string) (localDirListing, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return localDirListing{Dirs: localRoots()}, nil
	}
	if !filepath.IsAbs(dir) {
		return localDirListing{}, errors.New("path must be absolute")
	}
	dir = filepath.Clean(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return localDirListing{}, errors.New("cannot read directory")
	}
	out := localDirListing{Path: dir, Dirs: []localDirEntry{}}
	if parent := filepath.Dir(dir); parent != dir {
		out.Parent = parent
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || name == "System Volume Information" {
			continue
		}
		out.Dirs = append(out.Dirs, localDirEntry{Name: name, Path: filepath.Join(dir, name)})
	}
	sort.Slice(out.Dirs, func(i, j int) bool {
		return strings.ToLower(out.Dirs[i].Name) < strings.ToLower(out.Dirs[j].Name)
	})
	if len(out.Dirs) > localDirLimit {
		out.Dirs = out.Dirs[:localDirLimit]
	}
	return out, nil
}

// localRoots 根层：用户主目录 + Windows 各盘符（非 Windows 为 /）。
func localRoots() []localDirEntry {
	var roots []localDirEntry
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, localDirEntry{Name: home, Path: home})
	}
	if runtime.GOOS != "windows" {
		return append(roots, localDirEntry{Name: "/", Path: "/"})
	}
	for c := 'A'; c <= 'Z'; c++ {
		drive := string(c) + `:\`
		if info, err := os.Stat(drive); err == nil && info.IsDir() {
			roots = append(roots, localDirEntry{Name: drive, Path: drive})
		}
	}
	return roots
}

// createLocalDir 在 parent 下新建名为 name 的文件夹（只建一层；已存在返回 os.ErrExist）。
func createLocalDir(parent, name string) (string, error) {
	parent = strings.TrimSpace(parent)
	name = strings.TrimSpace(name)
	if parent == "" || !filepath.IsAbs(parent) {
		return "", errors.New("parent must be absolute")
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `<>:"/\|?*`) {
		return "", errors.New("invalid folder name")
	}
	parent = filepath.Clean(parent)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", errors.New("parent not found")
	}
	abs := filepath.Join(parent, name)
	if err := os.Mkdir(abs, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", os.ErrExist
		}
		return "", errors.New("cannot create folder")
	}
	return abs, nil
}
