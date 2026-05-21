package biz

func validateJSONSchema(schemaJSON, docJSON string) error {
	return validateDocumentAgainstSchema("PLUGIN", schemaJSON, docJSON)
}
