package metrics

import (
	"github.com/bmatcuk/doublestar/v4"
)

// BinaryPattern is a special junk pattern that matches every binary file instead of a path glob
const BinaryPattern = "<binary>"

var DefaultJunkPatterns = []string{
	"**/vendor/**",
	"**/node_modules/**",
	"**/dist/**",
	"**/build/**",
	"**/out/**",
	"**/target/**",
	"**/bin/**",
	"**/obj/**",
	"**/.next/**",
	"**/__pycache__/**",
	"**/*.class",
	"**/*.jar",
	"**/*.o",
	"**/*.so",
	"**/*.dll",
	"**/*.exe",
	"**/*.pyc",
	BinaryPattern,
}

// IsJunk treats an invalid pattern as one that matches nothing
func IsJunk(file SnapshotFile, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == BinaryPattern {
			if file.IsBinary {
				return true
			}

			continue
		}

		if isMatch, _ := doublestar.Match(pattern, file.Path); isMatch {
			return true
		}
	}

	return false
}
