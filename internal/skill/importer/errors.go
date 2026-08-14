package importer

import "errors"

var (
	ErrImportJobNotFound       = errors.New("import job not found")
	ErrConflictGroupNotFound   = errors.New("conflict group not found")
	ErrNoChatModelConfigured   = errors.New("no configured model with API base URL and API key is available")
	ErrCandidateNotFound       = errors.New("candidate not found")
	ErrCandidateNotPass        = errors.New("candidate is not pass")
	ErrRiskApprovalRequired    = errors.New("candidate does not require high risk approval")
	ErrUnsupportedAction       = errors.New("unsupported import action")
	ErrUnsafePathAbsolute      = errors.New("unsafe path: absolute")
	ErrUnsafePathTraversal     = errors.New("unsafe path: traversal")
	ErrUnsafePathEscapes       = errors.New("unsafe path: escapes base")
	ErrUnsafePathDotDot        = errors.New("unsafe path: dotdot")
	ErrSkillFileTooLarge       = errors.New("skill file too large")
	ErrTooManyFiles            = errors.New("skill zip contains too many files")
	ErrTotalSizeExceeded       = errors.New("skill zip uncompressed size exceeds limit")
	ErrChatCompletionFailed    = errors.New("chat completion failed")
	ErrEmptyChatResponse       = errors.New("empty chat completion response")
	ErrAnthropicFailed         = errors.New("anthropic messages failed")
	ErrEmptyAnthropicResponse  = errors.New("empty anthropic response")
	ErrRefineResultInvalid     = errors.New("model refine result missing merged_name or merged_body")
	ErrUnsafeRelPath           = errors.New("unsafe relative path in skill directory")
	ErrResolvePath             = errors.New("resolve skill file path")
	ErrRelPath                 = errors.New("rel skill file path")
	ErrResolveTargetDir        = errors.New("resolve target dir")
	ErrCandidateNotDuplicate   = errors.New("candidate is not duplicate-blocked")
	ErrDuplicateTargetNotFound = errors.New("duplicate target skill not found")
	ErrMergeSourceUnreadable   = errors.New("merge source skill unreadable")
)

type pathError struct {
	err    error
	detail string
}

func (e *pathError) Error() string { return e.detail + ": " + e.err.Error() }
func (e *pathError) Unwrap() error { return e.err }

func unsafePathError(sentinel error, detail string) error {
	return &pathError{err: sentinel, detail: detail}
}

type detailError struct {
	err    error
	detail string
}

func (e *detailError) Error() string { return e.err.Error() + ": " + e.detail }
func (e *detailError) Unwrap() error { return e.err }

func detailErr(sentinel error, detail string) error {
	return &detailError{err: sentinel, detail: detail}
}
