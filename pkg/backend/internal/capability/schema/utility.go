package schema

type EmptyInput struct{}

type DateTimeOutput struct {
	Local string `json:"local"`
	UTC   string `json:"utc"`
}

type WebFetchInput struct {
	URL string `json:"url" jsonschema:"public HTTP or HTTPS URL to fetch"`
}

type WebFetchOutput struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Text        string `json:"text"`
}
