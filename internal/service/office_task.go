package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/office/v1"
	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"

	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 个人任务 RPC（工作台「我的任务」「进行中」与随手记录）。只读写调用者本人的任务，属主来自
// 登录上下文，请求体不接受 user_id。

func officeTaskToProto(t *office.OfficeTask) *v1.OfficeTask {
	out := &v1.OfficeTask{
		Id:           t.ID,
		Title:        t.Title,
		Body:         t.Body,
		Source:       t.Source,
		SourceRef:    t.SourceRef,
		Status:       t.Status,
		AssigneeKind: t.AssigneeKind,
		AssigneeId:   t.AssigneeID,
		AssigneeName: t.AssigneeName,
		SessionId:    t.SessionID,
		FileCount:    int32(t.FileCount),
		CreatedAt:    timestamppb.New(t.CreatedAt),
		UpdatedAt:    timestamppb.New(t.UpdatedAt),
	}
	if t.DueAt != nil {
		out.DueAt = timestamppb.New(*t.DueAt)
	}
	return out
}

func officeTaskFileToProto(f *office.OfficeTaskFile) *v1.OfficeTaskFile {
	return &v1.OfficeTaskFile{
		Id:          f.ID,
		TaskId:      f.TaskID,
		Filename:    f.Filename,
		ContentType: f.ContentType,
		Size:        f.Size,
		CreatedAt:   timestamppb.New(f.CreatedAt),
	}
}

// dueFromProto 空时间戳 = 没设。
func dueFromProto(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil || (ts.GetSeconds() == 0 && ts.GetNanos() == 0) {
		return nil
	}
	at := ts.AsTime()
	return &at
}

func (s *OfficeMailService) ListOfficeTasks(ctx context.Context, req *v1.ListOfficeTasksRequest) (*v1.ListOfficeTasksResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	rows, err := s.messages.ListTasks(ctx, userID, req.GetStatus(), req.GetSource(), int(req.GetLimit()))
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListOfficeTasksResponse{Items: make([]*v1.OfficeTask, 0, len(rows))}
	for _, t := range rows {
		out.Items = append(out.Items, officeTaskToProto(t))
	}
	return out, nil
}

func (s *OfficeMailService) CreateOfficeTask(ctx context.Context, req *v1.CreateOfficeTaskRequest) (*v1.OfficeTask, error) {
	userID, workspaceID, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.CreateTask(ctx, userID, workspaceID, office.NewOfficeTaskInput{
		Title:        req.GetTitle(),
		Body:         req.GetBody(),
		Source:       req.GetSource(),
		SourceRef:    req.GetSourceRef(),
		AssigneeKind: req.GetAssigneeKind(),
		AssigneeID:   req.GetAssigneeId(),
		AssigneeName: req.GetAssigneeName(),
		DueAt:        dueFromProto(req.GetDueAt()),
	})
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}

func (s *OfficeMailService) SetOfficeTaskStatus(ctx context.Context, req *v1.SetOfficeTaskStatusRequest) (*v1.OfficeTask, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.SetTaskStatus(ctx, req.GetId(), userID, req.GetStatus())
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}

func (s *OfficeMailService) UpdateOfficeTask(ctx context.Context, req *v1.UpdateOfficeTaskRequest) (*v1.OfficeTask, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.UpdateTask(ctx, req.GetId(), userID, office.UpdateOfficeTaskInput{
		Title:    req.Title,
		Body:     req.Body,
		DueAt:    dueFromProto(req.GetDueAt()),
		ClearDue: req.GetClearDue(),
	})
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}

func (s *OfficeMailService) AssignOfficeTask(ctx context.Context, req *v1.AssignOfficeTaskRequest) (*v1.OfficeTask, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.AssignTask(ctx, req.GetId(), userID, office.AssignOfficeTaskInput{
		Kind:      req.GetAssigneeKind(),
		ID:        req.GetAssigneeId(),
		Name:      req.GetAssigneeName(),
		SessionID: req.GetSessionId(),
	})
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}

func (s *OfficeMailService) DeleteOfficeTask(ctx context.Context, req *v1.DeleteOfficeTaskRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	if err := s.messages.DeleteTask(ctx, req.GetId(), userID); err != nil {
		return nil, mailErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeMailService) ListOfficeTaskFiles(ctx context.Context, req *v1.ListOfficeTaskFilesRequest) (*v1.ListOfficeTaskFilesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	rows, err := s.messages.ListTaskFiles(ctx, req.GetTaskId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListOfficeTaskFilesResponse{Items: make([]*v1.OfficeTaskFile, 0, len(rows))}
	for _, f := range rows {
		out.Items = append(out.Items, officeTaskFileToProto(f))
	}
	return out, nil
}

func (s *OfficeMailService) UploadOfficeTaskFile(ctx context.Context, req *v1.UploadOfficeTaskFileRequest) (*v1.OfficeTaskFile, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	f, err := s.messages.UploadTaskFile(ctx, req.GetTaskId(), userID, req.GetFilename(), req.GetContentType(), req.GetContent())
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskFileToProto(f), nil
}

// DownloadOfficeTaskFile 附件原始字节（HttpBody 直通，不 JSON 包裹）。
func (s *OfficeMailService) DownloadOfficeTaskFile(ctx context.Context, req *v1.OfficeTaskFileRequest) (*httpbody.HttpBody, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	f, err := s.messages.GetTaskFile(ctx, req.GetId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	return &httpbody.HttpBody{ContentType: safeDownloadType(f.ContentType), Data: f.Content}, nil
}

// safeDownloadType 下载与 API 同源：能在浏览器里执行脚本的类型（HTML / SVG / XML）一律按
// 二进制流给出，直接打开链接也只会下载，不会在本站源下渲染。
func safeDownloadType(ct string) string {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	switch {
	case base == "", strings.Contains(base, "html"), strings.Contains(base, "svg"),
		strings.Contains(base, "xml"), strings.Contains(base, "javascript"):
		return "application/octet-stream"
	}
	return ct
}

func (s *OfficeMailService) DeleteOfficeTaskFile(ctx context.Context, req *v1.OfficeTaskFileRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	if err := s.messages.DeleteTaskFile(ctx, req.GetId(), userID); err != nil {
		return nil, mailErr(err)
	}
	return &emptypb.Empty{}, nil
}

// bindOfficeTaskFileMultipart 把 multipart/form-data（字段 file）转成上传请求，
// 省掉 JSON base64 的三分之一膨胀。task_id 来自路径，由 kratos 在解码后绑定。
// 读满上限 +1 字节交给 biz 判超限，避免这里静默截断。
func bindOfficeTaskFileMultipart(r *http.Request, req *v1.UploadOfficeTaskFileRequest) error {
	r.Body = http.MaxBytesReader(nil, r.Body, office.OfficeTaskFileMaxBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return apierror.ToKratos(office.ErrOfficeTaskFileTooLarge)
		}
		return apierror.ToKratos(apierror.BadRequest(office.DomainOffice, "invalid multipart upload"))
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, office.OfficeTaskFileMaxBytes+1))
	if err != nil {
		return err
	}
	req.Content = data
	req.Filename = strings.TrimSpace(r.FormValue("filename"))
	if req.Filename == "" && header != nil {
		req.Filename = header.Filename
	}
	if header != nil {
		req.ContentType = header.Header.Get("Content-Type")
	}
	return nil
}
