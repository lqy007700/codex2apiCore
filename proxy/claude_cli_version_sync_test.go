package proxy

import (
	"testing"
)

func withClaudeVersionSources(t *testing.T, github, npm string) {
	t.Helper()
	claudeReleasesLatestURLForTest = github
	claudeNpmDistTagsURLForTest = npm
	t.Cleanup(func() {
		claudeReleasesLatestURLForTest = ""
		claudeNpmDistTagsURLForTest = ""
	})
}
