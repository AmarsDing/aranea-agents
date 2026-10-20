package knowledge

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// DocumentACLStore 文档可见性写口（可选接线；未接线时 UpdateDocumentVisibility 不可用）。
// sharedWith 仅 visibility=shared 时有意义（其余档位实现方须清空，见 usecase 语义）。
type DocumentACLStore interface {
	UpdateDocumentVisibility(ctx context.Context, id, visibility, ownerUserID string, sharedWith []string) error
}

// OrgNodeValidator 校验共享名单内的 org 节点（G-08）：每个 ID 须为组织树上
// 存在、未软删、且 level ∈ {department, position} 的节点（company 级共享等同
// collection，拒收）。可选接线；未接线时 visibility=shared 写入不可用。
type OrgNodeValidator interface {
	ValidateSharedOrgNodes(ctx context.Context, orgNodeIDs []string) error
}

func (u *Usecase) SetDocumentACLStore(s DocumentACLStore) {
	if u != nil {
		u.docACL = s
	}
}

func (u *Usecase) SetOrgNodeValidator(v OrgNodeValidator) {
	if u != nil {
		u.orgValidator = v
	}
}

func viewerUserID(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return ""
}

// DocumentVisibleTo reports whether ctx may read d. System sees all.
// collection (or empty) is visible to anyone with collection access; private
// is owner-only; shared (G-08) is owner + 组织可见域 ∩ SharedWith ≠ ∅（人类
// 用户无 org 绑定，对其仅属主可见——fail-closed；未知档位一律拒绝）。
func DocumentVisibleTo(ctx context.Context, d Document) bool {
	if workspace.IsSystem(ctx) {
		return true
	}
	vis := strings.TrimSpace(d.Visibility)
	if vis == "" || vis == DocVisibilityCollection {
		return true
	}
	owner := strings.TrimSpace(d.OwnerUserID)
	uid := viewerUserID(ctx)
	switch vis {
	case DocVisibilityPrivate:
		return owner != "" && owner == uid
	case DocVisibilityShared:
		if owner != "" && owner == uid {
			return true
		}
		return OrgScopeIntersects(OrgScopeFromContext(ctx), d.SharedWith)
	default:
		return false
	}
}

func (u *Usecase) UpdateDocumentVisibility(ctx context.Context, id, visibility string, sharedWith []string) (Document, error) {
	if u == nil || u.documents == nil {
		return Document{}, ErrUnavailable
	}
	if u.docACL == nil {
		return Document{}, apierror.Unavailable("KNOWLEDGE", "document visibility is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Document{}, ErrIDRequired
	}
	doc, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, err
	}
	visibility = strings.TrimSpace(strings.ToLower(visibility))
	if visibility == "" {
		visibility = DocVisibilityCollection
	}
	switch visibility {
	case DocVisibilityCollection, DocVisibilityPrivate:
		sharedWith = nil // 非 shared 档清空名单：切档不留隐藏状态
	case DocVisibilityShared:
		sharedWith = normalizeOrgScope(sharedWith)
		if len(sharedWith) == 0 {
			return Document{}, apierror.BadRequest("KNOWLEDGE", "visibility=shared requires a non-empty shared_with list")
		}
		if u.orgValidator == nil {
			return Document{}, apierror.Unavailable("KNOWLEDGE", "org node validation is not configured")
		}
		if err := u.orgValidator.ValidateSharedOrgNodes(ctx, sharedWith); err != nil {
			return Document{}, err
		}
	default:
		return Document{}, apierror.BadRequest("KNOWLEDGE", "visibility must be collection, private or shared")
	}
	uid := viewerUserID(ctx)
	owner := strings.TrimSpace(doc.OwnerUserID)
	// G-03 写侧归属校验：仅 owner / admin / system 可改变可见性；owner 为空
	// 视为共享资产，仅 admin/system 可收编。非授权者按统一 IDOR 遮蔽模式返回
	// NotFound（不泄露"文档存在但你无权"这一信息）。
	if !workspace.IsSystem(ctx) {
		admin := false
		if a, ok := auth.FromContext(ctx); ok && a != nil {
			admin = a.HasAdminAccess()
		}
		if !admin {
			if uid == "" || owner == "" || owner != uid {
				return Document{}, apierror.NotFound("KNOWLEDGE", "document not found: %s", id)
			}
		}
	}
	switch visibility {
	case DocVisibilityPrivate:
		if uid == "" {
			return Document{}, apierror.BadRequest("KNOWLEDGE", "sign in required to mark a document private")
		}
		// 仅收编（owner 从空到有）或本人操作时才写 owner；admin/system
		// 改动他人文档时保留原 owner，避免"管理员私有化即夺产"的审计失真。
		if owner == "" || owner == uid {
			owner = uid
		}
	case DocVisibilityCollection, DocVisibilityShared:
		// keep owner for audit; visibility change is the ACL switch
	}
	if err := u.docACL.UpdateDocumentVisibility(ctx, id, visibility, owner, sharedWith); err != nil {
		return Document{}, err
	}
	doc.Visibility = visibility
	doc.OwnerUserID = owner
	doc.SharedWith = sharedWith
	return doc, nil
}
