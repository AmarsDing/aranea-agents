package schema

type FilePathInput struct {
	Path string `json:"path" jsonschema:"path relative to the workspace root"`
}

type ReadFileOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
}

type FileListOutput struct {
	Path  string         `json:"path"`
	Items []FileListItem `json:"items"`
}

type FileListItem struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"modTime,omitempty"`
}

type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"path relative to the workspace root"`
	Content string `json:"content" jsonschema:"UTF-8 file content to write"`
	Deliver bool   `json:"deliver,omitempty" jsonschema:"whether the written file should be surfaced to the user"`
}

type WriteFileOutput struct {
	Path    string `json:"path"`
	Written int    `json:"written"`
}

type EditFileInput struct {
	Path      string `json:"path" jsonschema:"path relative to the workspace root"`
	OldString string `json:"old_string" jsonschema:"exact text occurrence to replace"`
	NewString string `json:"new_string" jsonschema:"replacement text"`
}

type EditFileOutput struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
}
