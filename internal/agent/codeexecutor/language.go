package codeexecutor

// languageRuntime returns the file extension and interpreter command for a language.
func languageRuntime(lang string) (ext, runner string) {
	switch lang {
	case "python", "python3":
		return ".py", "python3"
	case "javascript", "js", "node":
		return ".js", "node"
	case "bash", "sh", "shell":
		return ".sh", "bash"
	case "ruby":
		return ".rb", "ruby"
	default:
		return "", ""
	}
}
