package utils

import "sort"

// SortedArgKeys returns the sorted argument names of a tool call for safe
// logging. Values are intentionally omitted because tool arguments can contain
// credentials, file contents, or shell commands with embedded secrets.
func SortedArgKeys(args map[string]any) []string {
	if len(args) == 0 {
		return nil
	}

	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
