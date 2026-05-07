package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/genai"

	"google.golang.org/adk/artifact"
)

// LocalArtifactService stores artifacts under {workspace}/.aranea-artifacts/{app}/{user}/{session}/.
type LocalArtifactService struct {
	mu     sync.RWMutex
	root   string
	created bool
}

var _ artifact.Service = (*LocalArtifactService)(nil)

// NewLocalArtifactService uses rootDir as base; if empty, uses os.TempDir()/aranea-artifacts.
func NewLocalArtifactService(rootDir string) *LocalArtifactService {
	r := strings.TrimSpace(rootDir)
	if r == "" {
		r = filepath.Join(os.TempDir(), "aranea-artifacts")
	}
	return &LocalArtifactService{root: r}
}

func (s *LocalArtifactService) dir(app, user, session string) (string, error) {
	if err := ensureArtifactDir(s.root); err != nil {
		return "", err
	}
	p := filepath.Join(s.root, safeSeg(app), safeSeg(user), safeSeg(session))
	if err := ensureArtifactDir(p); err != nil {
		return "", err
	}
	return p, nil
}

func ensureArtifactDir(p string) error {
	return os.MkdirAll(p, 0o755)
}

func safeSeg(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "_"
	}
	s = strings.ReplaceAll(s, string(filepath.Separator), "_")
	s = strings.ReplaceAll(s, "..", "_")
	return s
}

func (s *LocalArtifactService) Save(ctx context.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	dir, err := s.dir(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, req.FileName)
	var data []byte
	if req.Part.InlineData != nil && len(req.Part.InlineData.Data) > 0 {
		data = req.Part.InlineData.Data
	} else if req.Part.Text != "" {
		data = []byte(req.Part.Text)
	} else {
		return nil, fmt.Errorf("artifact: empty part")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return &artifact.SaveResponse{Version: 1}, nil
}

func (s *LocalArtifactService) Load(ctx context.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	dir, err := s.dir(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, req.FileName))
	if err != nil {
		return nil, err
	}
	return &artifact.LoadResponse{
		Part: &genai.Part{Text: string(b)},
	}, nil
}

func (s *LocalArtifactService) Delete(ctx context.Context, req *artifact.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	dir, err := s.dir(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, req.FileName))
}

func (s *LocalArtifactService) List(ctx context.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	dir, err := s.dir(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return &artifact.ListResponse{FileNames: names}, nil
}

func (s *LocalArtifactService) Versions(ctx context.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	dir, err := s.dir(req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, req.FileName)
	if _, err := os.Stat(path); err != nil {
		return &artifact.VersionsResponse{Versions: nil}, nil
	}
	return &artifact.VersionsResponse{Versions: []int64{1}}, nil
}

func (s *LocalArtifactService) GetArtifactVersion(ctx context.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return &artifact.GetArtifactVersionResponse{
		ArtifactVersion: &artifact.ArtifactVersion{Version: 1, MimeType: "application/octet-stream"},
	}, nil
}
