package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/secops/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// newGateProbeService 装配一个「零业务依赖」secops service：闸门测试只关心
// 到达 usecase 之后的判定（Unauthorized/Forbidden/BadRequest/输入校验），
// biz 层对 nil repo 有守卫（Internal），不会触到空指针；offboard 正路径
// 由 data 层五步链路测试覆盖，D6 正路径由 biz/data 测试覆盖。
func newGateProbeService() *SecopsService {
	uc := biz.NewSecopsOffboardUsecase(nil, nil, nil, nil, nil, loggateway.NewNoop())
	classification := biz.NewSecopsClassificationUsecase(nil, nil, loggateway.NewNoop())
	policy := biz.NewSecopsModelPolicyUsecase(nil, loggateway.NewNoop())
	return NewSecopsService(uc, classification, policy, nil, loggateway.NewNoop())
}

func offboardCtx(uid int64, roles ...string) context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: uid, Roles: roles})
}

// TestSecopsService_RoleGates 钉死 service 兜底闸门（middleware 登记表的
// 第二道保险）：
//   - 未认证 → Unauthorized（任何 RPC）；
//   - auditor（仅 audit.*）→ Forbidden：user.offboard 缺失；
//   - secadmin（有 secops.policy 但无 user.offboard 角色）→ Forbidden：
//     需求钉死「审批/导出仅 sysadmin」，secops.policy 不构成旁路；
//   - approve/export 的角色硬门：sysadmin 码位持有 ≠ sysadmin 角色（本测试
//     中 superadmin 过门后以 BadRequest 证明门已通过）；
//   - user_id 解析：非正整数 → BadRequest。
func TestSecopsService_RoleGates(t *testing.T) {
	svc := newGateProbeService()
	noAuth := context.Background()

	cases := []struct {
		name string
		ctx  context.Context
		call func(context.Context) error
		code apierror.Code
	}{
		{"freeze no auth", noAuth, func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "7"})
			return err
		}, apierror.CodeUnauthorized},
		{"freeze auditor lacks user.offboard", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "7"})
			return err
		}, apierror.CodeForbidden},
		{"freeze secadmin no bypass via secops.policy", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "7"})
			return err
		}, apierror.CodeForbidden},
		{"request secadmin forbidden", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.RequestOffboardExport(c, &v1.RequestOffboardExportRequest{UserId: "7"})
			return err
		}, apierror.CodeForbidden},
		{"approve secadmin role gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.ApproveOffboardExport(c, &v1.ApproveOffboardExportRequest{UserId: "7", Approve: true})
			return err
		}, apierror.CodeForbidden},
		{"approve auditor role gate", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.ApproveOffboardExport(c, &v1.ApproveOffboardExportRequest{UserId: "7", Approve: true})
			return err
		}, apierror.CodeForbidden},
		{"export secadmin role gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.GetOffboardExport(c, &v1.GetOffboardExportRequest{UserId: "7"})
			return err
		}, apierror.CodeForbidden},
		{"delete auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.DeleteOffboardedUser(c, &v1.DeleteOffboardedUserRequest{UserId: "7"})
			return err
		}, apierror.CodeForbidden},
		{"list auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.ListOffboardRequests(c, nil)
			return err
		}, apierror.CodeForbidden},
		// sysadmin 持有 user.offboard → 过权限门，user_id 解析失败给出 BadRequest
		//（证明未在权限门被拦）。
		{"freeze sysadmin bad uid alpha", offboardCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "abc"})
			return err
		}, apierror.CodeBadRequest},
		{"freeze sysadmin uid zero", offboardCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "0"})
			return err
		}, apierror.CodeBadRequest},
		{"freeze sysadmin uid negative", offboardCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.FreezeUser(c, &v1.FreezeUserRequest{UserId: "-5"})
			return err
		}, apierror.CodeBadRequest},
		{"approve superadmin passes role gate", offboardCtx(99, "superadmin"), func(c context.Context) error {
			_, err := svc.ApproveOffboardExport(c, &v1.ApproveOffboardExportRequest{UserId: "xyz", Approve: true})
			return err
		}, apierror.CodeBadRequest},
		{"export superadmin passes role gate", offboardCtx(99, "superadmin"), func(c context.Context) error {
			_, err := svc.GetOffboardExport(c, &v1.GetOffboardExportRequest{UserId: ""})
			return err
		}, apierror.CodeBadRequest},
		{"request sysadmin bad uid", offboardCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.RequestOffboardExport(c, &v1.RequestOffboardExportRequest{UserId: "  "})
			return err
		}, apierror.CodeBadRequest},
		{"delete sysadmin bad uid", offboardCtx(71, "sysadmin"), func(c context.Context) error {
			_, err := svc.DeleteOffboardedUser(c, &v1.DeleteOffboardedUserRequest{UserId: "7x"})
			return err
		}, apierror.CodeBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.ctx)
			if !apierror.IsCode(err, tc.code) {
				t.Fatalf("want code %s, got %v", tc.code, err)
			}
		})
	}
}

// TestSecopsService_Unwired 防护：offboard usecase 未装配时任何 RPC 返回
// Internal（nil 服务在 wire 中已被 provideSecopsService 的 nil-check 拦截，
// 这里钉死结构体层面的兜底）。
func TestSecopsService_Unwired(t *testing.T) {
	svc := &SecopsService{}
	ctx := offboardCtx(71, "sysadmin")
	if _, err := svc.FreezeUser(ctx, &v1.FreezeUserRequest{UserId: "7"}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired freeze: %v", err)
	}
	if _, err := svc.ListOffboardRequests(ctx, nil); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("unwired list: %v", err)
	}
}

// TestSecopsService_D6PolicyGates 钉死 D6 密级/策略四 RPC 的兜底闸门序：
//   - 未认证 → Unauthorized（任何 RPC）；
//   - 标签列表仅要求认证态：认证 + nil usecase → Internal（证明过闸门）；
//   - 标签写/策略读写要求 secops.policy：auditor/viewer → Forbidden；
//     secadmin 持 secops.policy → 过权限门后以 Internal（nil usecase）
//     证明门已通过——sysadmin 同样过门（两角色均映射该码位）。
func TestSecopsService_D6PolicyGates(t *testing.T) {
	svc := newGateProbeService()
	noAuth := context.Background()

	cases := []struct {
		name string
		ctx  context.Context
		call func(context.Context) error
		code apierror.Code
	}{
		{"labels list no auth", noAuth, func(c context.Context) error {
			_, err := svc.ListClassificationLabels(c, nil)
			return err
		}, apierror.CodeUnauthorized},
		{"labels list authenticated passes gate", offboardCtx(55, "viewer"), func(c context.Context) error {
			_, err := svc.ListClassificationLabels(c, nil)
			return err
		}, apierror.CodeInternal},
		{"put label auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.PutClassificationLabel(c, &v1.PutClassificationLabelRequest{Level: 3, Name: "x"})
			return err
		}, apierror.CodeForbidden},
		{"put label viewer forbidden", offboardCtx(52, "viewer"), func(c context.Context) error {
			_, err := svc.PutClassificationLabel(c, &v1.PutClassificationLabelRequest{Level: 3, Name: "x"})
			return err
		}, apierror.CodeForbidden},
		{"put label secadmin passes permission gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			// level 9 越界：过权限门后由 biz 输入校验给出 BadRequest（证明门已过）。
			_, err := svc.PutClassificationLabel(c, &v1.PutClassificationLabelRequest{Level: 9, Name: "x"})
			return err
		}, apierror.CodeBadRequest},
		{"get policy auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.GetModelPolicy(c, nil)
			return err
		}, apierror.CodeForbidden},
		{"get policy secadmin passes permission gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.GetModelPolicy(c, nil)
			return err
		}, apierror.CodeInternal},
		{"set policy viewer forbidden", offboardCtx(52, "viewer"), func(c context.Context) error {
			_, err := svc.SetModelPolicy(c, &v1.SetModelPolicyRequest{LocalThreshold: 3})
			return err
		}, apierror.CodeForbidden},
		// 实锚：secops.policy 码位由 secadmin 持有（sysadmin 不持有——需求
		// §7.3「仅 sysadmin 可改」与 W0 proto 登记的语义差异登记 W4 评审项 (u)）。
		{"set policy secadmin passes permission gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.SetModelPolicy(c, &v1.SetModelPolicyRequest{LocalThreshold: 3})
			return err
		}, apierror.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.ctx)
			if !apierror.IsCode(err, tc.code) {
				t.Fatalf("want code %s, got %v", tc.code, err)
			}
		})
	}
}

// TestSecopsService_D6MaskGates 钉死脱敏五闸七 RPC 的兜底闸门序（D6 V4）：
//   - 未认证 → Unauthorized；
//   - 全组要求 secops.policy：auditor/viewer → Forbidden；
//   - secadmin 持 secops.policy → 过权限门后以 Internal（nil usecase）证明
//     门已通过。
func TestSecopsService_D6MaskGates(t *testing.T) {
	svc := newGateProbeService()
	noAuth := context.Background()

	cases := []struct {
		name string
		ctx  context.Context
		call func(context.Context) error
		code apierror.Code
	}{
		{"rules list no auth", noAuth, func(c context.Context) error {
			_, err := svc.ListMaskRules(c, nil)
			return err
		}, apierror.CodeUnauthorized},
		{"rules list viewer forbidden", offboardCtx(52, "viewer"), func(c context.Context) error {
			_, err := svc.ListMaskRules(c, nil)
			return err
		}, apierror.CodeForbidden},
		{"rules list secadmin passes gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.ListMaskRules(c, nil)
			return err
		}, apierror.CodeInternal},
		{"proposals list auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.ListMaskProposals(c, &v1.ListMaskProposalsRequest{})
			return err
		}, apierror.CodeForbidden},
		{"create proposal no auth", noAuth, func(c context.Context) error {
			_, err := svc.CreateMaskProposal(c, &v1.CreateMaskProposalRequest{Entity: "手机号"})
			return err
		}, apierror.CodeUnauthorized},
		{"create proposal viewer forbidden", offboardCtx(52, "viewer"), func(c context.Context) error {
			_, err := svc.CreateMaskProposal(c, &v1.CreateMaskProposalRequest{Entity: "手机号"})
			return err
		}, apierror.CodeForbidden},
		{"create proposal secadmin passes gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.CreateMaskProposal(c, &v1.CreateMaskProposalRequest{Entity: "手机号"})
			return err
		}, apierror.CodeInternal},
		{"run gates auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.RunMaskGates(c, &v1.MaskProposalActionRequest{Id: "mp_1"})
			return err
		}, apierror.CodeForbidden},
		{"approve auditor forbidden", offboardCtx(91, "auditor"), func(c context.Context) error {
			_, err := svc.ApproveMaskProposal(c, &v1.MaskProposalActionRequest{Id: "mp_1"})
			return err
		}, apierror.CodeForbidden},
		{"reject viewer forbidden", offboardCtx(52, "viewer"), func(c context.Context) error {
			_, err := svc.RejectMaskProposal(c, &v1.MaskProposalActionRequest{Id: "mp_1"})
			return err
		}, apierror.CodeForbidden},
		{"activate no auth", noAuth, func(c context.Context) error {
			_, err := svc.ActivateMaskProposal(c, &v1.ActivateMaskProposalRequest{Id: "mp_1", RolloutPct: 10})
			return err
		}, apierror.CodeUnauthorized},
		{"activate secadmin passes gate", offboardCtx(81, "secadmin"), func(c context.Context) error {
			_, err := svc.ActivateMaskProposal(c, &v1.ActivateMaskProposalRequest{Id: "mp_1", RolloutPct: 10})
			return err
		}, apierror.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.ctx)
			if !apierror.IsCode(err, tc.code) {
				t.Fatalf("want code %s, got %v", tc.code, err)
			}
		})
	}
}
