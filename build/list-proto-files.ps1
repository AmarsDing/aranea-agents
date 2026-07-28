param([string]$Dir = "api")
# List *.proto files under $Dir as relative paths with forward slashes (one per line).
# Used by Makefile $(shell ...) on Windows where Git-Bash/sh is unavailable.
Get-ChildItem -Path $Dir -Recurse -Filter *.proto -File | ForEach-Object {
    $_.FullName.Substring((Get-Location).Path.Length + 1).Replace('\', '/')
}
