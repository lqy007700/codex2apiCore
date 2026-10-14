package proxy

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestGrokPrefixFingerprintIgnoresLaterUserTurns(t *testing.T) {
	a := []byte(`{"tools":[{"type":"function","name":"Read"}],"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"static"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]}]}`)
	b := []byte(`{"tools":[{"type":"function","name":"Read"}],"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"static"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"q2"}]}]}`)
	c := []byte(`{"tools":[{"type":"function","name":"Write"}],"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"static"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"q1"}]}]}`)
	if grokShortHash(gjson.GetBytes(a, "tools").Raw) != grokShortHash(gjson.GetBytes(b, "tools").Raw) {
		t.Fatal("same tools should share tools hash")
	}
	if grokShortHash(gjson.GetBytes(a, "tools").Raw) == grokShortHash(gjson.GetBytes(c, "tools").Raw) {
		t.Fatal("different tools must not share tools hash")
	}
}

func grokToolNames(tools []any) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, grokToolName(tool))
	}
	return out
}
