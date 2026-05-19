package biz

import (
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/xeipuuv/gojsonschema"
)

func validateJSONSchema(schemaJSON, docJSON string) error {
	schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
	docLoader := gojsonschema.NewStringLoader(docJSON)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return errors.InternalServer("PLUGIN", "schema validation error: "+err.Error())
	}
	if !result.Valid() {
		var msgs []string
		for _, desc := range result.Errors() {
			msgs = append(msgs, desc.String())
		}
		return errors.BadRequest("PLUGIN", "config does not match schema: "+strings.Join(msgs, "; "))
	}
	return nil
}
