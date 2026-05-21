package biz

import (
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/xeipuuv/gojsonschema"
)

func validateDocumentAgainstSchema(module, schemaJSON, docJSON string) error {
	schemaJSON = strings.TrimSpace(schemaJSON)
	docJSON = strings.TrimSpace(docJSON)
	if schemaJSON == "" || schemaJSON == "{}" {
		return nil
	}
	if docJSON == "" {
		docJSON = "{}"
	}
	result, err := gojsonschema.Validate(
		gojsonschema.NewStringLoader(schemaJSON),
		gojsonschema.NewStringLoader(docJSON),
	)
	if err != nil {
		return kerrors.InternalServer(module, "schema validation error: "+err.Error())
	}
	if result.Valid() {
		return nil
	}
	var msgs []string
	for _, desc := range result.Errors() {
		msgs = append(msgs, desc.String())
	}
	return kerrors.BadRequest(module, "config does not match schema: "+strings.Join(msgs, "; "))
}
