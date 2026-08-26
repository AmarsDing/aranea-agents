package validate

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Middleware is a middleware that validates the request message with [FieldBehavior](https://google.aip.dev/203)
func Middleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (reply any, err error) {
			if msg, ok := req.(proto.Message); ok {
				if err := ValidateRequiredFields(msg); err != nil {
					return nil, errors.BadRequest("VALIDATOR", err.Error()).WithCause(err)
				}
			}
			return handler(ctx, req)
		}
	}
}

// ValidateRequiredFields returns a validation error if any field annotated as
// required does not have a value. See: https://aip.dev/203
//
// Presence 语义（2026-08-27 验收3 根修）：用 protoreflect Message.Has 判定，
// 取代 go.einride.tech/aip v0.76.0 fieldbehavior.ValidateRequiredFields 的值
// 判定（isPresent 对 BoolKind 直接 return v.Bool()）。Has 的语义：
//   - proto3 optional / message / oneof 成员：显式设置即 present——optional
//     bool 显式 false（HITL 拒绝）合法，不会被误判 "missing required field"。
//   - proto3 隐式 presence 标量：非零值即 present（与库旧行为一致）。
//   - repeated / map：len > 0 即 present（与库旧行为一致）。
func ValidateRequiredFields(m proto.Message) error {
	return validateRequiredFields(m.ProtoReflect(), "")
}

func validateRequiredFields(m protoreflect.Message, path string) error {
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		currPath := path
		if len(currPath) > 0 {
			currPath += "."
		}
		currPath += string(field.Name())
		if !m.Has(field) {
			if hasFieldBehavior(field, annotations.FieldBehavior_REQUIRED) {
				return fmt.Errorf("missing required field: %s", currPath)
			}
			continue
		}
		// 仅对 present 的消息字段递归（repeated 元素 / map 值 / 单条消息），
		// 与 einride fieldbehavior 的遍历范围保持一致。
		if field.Kind() != protoreflect.MessageKind {
			continue
		}
		value := m.Get(field)
		switch {
		case field.IsList():
			for j := 0; j < value.List().Len(); j++ {
				if err := validateRequiredFields(value.List().Get(j).Message(), currPath); err != nil {
					return err
				}
			}
		case field.IsMap():
			var mapErr error
			value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
				if v.Message().IsValid() {
					mapErr = validateRequiredFields(v.Message(), currPath)
				}
				return mapErr == nil
			})
			if mapErr != nil {
				return mapErr
			}
		default:
			if err := validateRequiredFields(value.Message(), currPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasFieldBehavior reports whether the field carries the wanted behavior.
func hasFieldBehavior(field protoreflect.FieldDescriptor, want annotations.FieldBehavior) bool {
	behaviors, ok := proto.GetExtension(
		field.Options(), annotations.E_FieldBehavior,
	).([]annotations.FieldBehavior)
	if !ok {
		return false
	}
	for _, got := range behaviors {
		if got == want {
			return true
		}
	}
	return false
}
