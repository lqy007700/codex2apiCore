package proxy

import (
	"bytes"

	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestOutputCollectorIdentityHasBoundedStorage(t *testing.T) {
	for _, id := range []string{"message-id", "", strings.Repeat("id", 1<<18)} {
		raw := fmt.Sprintf(`{"id":%q,"type":"message","content":%q}`, id, strings.Repeat("x", 1<<20))
		key := responseOutputItemDoneKey(gjson.Parse(raw))
		if len(key) > 300 {
			t.Fatalf("identity retained body: %d bytes", len(key))
		}
		if key != responseOutputItemDoneKey(gjson.Parse(raw)) {
			t.Fatal("unstable identity")
		}
	}
}

func TestOutputCollectorOverflowDropsAllRetainedIndexes(t *testing.T) {
	collector := newResponseOutputCollector()
	collector.limit = 256
	event := []byte(fmt.Sprintf(`{"type":"response.output_item.done","output_index":0,"item":{"id":"message-1","type":"message","content":%q}}`, strings.Repeat("x", 512)))
	if collector.Add(event) || !collector.overflow || collector.seen != nil || collector.indexed != nil || collector.unindexed != nil || collector.bytes != 0 {
		t.Fatalf("overflow retains indexes: %+v", collector)
	}
	if collector.Add([]byte(`{"type":"response.output_item.done","item":{"id":"late"}}`)) {
		t.Fatal("overflow resumed collection")
	}
}

func TestOutputCollectorBoundsReplacementIdentities(t *testing.T) {
	collector := newResponseOutputCollector()
	for i := 0; i < responseOutputCollectorMaxItems; i++ {
		collector.Add([]byte(fmt.Sprintf(`{"type":"response.output_item.done","output_index":0,"item":{"id":"message-%d","type":"message","content":"x"}}`, i)))
	}
	for _, event := range []string{
		`{"type":"response.completed","response":{"output":[]}}`,
		`{"type":"response.output_text.delta","delta":"next"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"message-0","type":"message","content":"x"}}`,
	} {
		collector.Add([]byte(event))
		if collector.overflow || len(collector.Items()) != 1 {
			t.Fatal("non-new item overflowed collector at exact limit")
		}
	}
	collector.Add([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"one-too-many","type":"message","content":"x"}}`))
	if !collector.overflow || len(collector.seen) != 0 {
		t.Fatalf("unbounded identities: %+v", collector)
	}
}

// If append grew the backing array, the earlier slice is independently owned.
// Re-run with reserved capacity so this observes the actual eviction slot.

func TestLargeStreamBuffersReleaseCapacityAndPreserveOutput(t *testing.T) {
	payload := []byte(strings.Repeat("x", 1<<20))
	var output bytes.Buffer
	w := &streamFlushWriter{writer: &output, policy: StreamFlushPolicyCoalesce, interval: 20 * time.Millisecond}
	if err := w.WriteBytes(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), payload) || w.buffer.Cap() != 0 {
		t.Fatalf("flush output=%d capacity=%d", output.Len(), w.buffer.Cap())
	}
	replay := newContinuousRetryReplayWithLimits(int64(len(payload)), 2*int64(len(payload)))
	defer replay.Close()
	if _, err := replay.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Write([]byte("tail")); err != nil {
		t.Fatal(err)
	}
	if replay.file == nil || replay.memory.Cap() != 0 {
		t.Fatal("spill retained memory buffer")
	}
	var restored bytes.Buffer
	if err := replay.CommitTo(&restored, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.Bytes(), append(append([]byte(nil), payload...), []byte("tail")...)) {
		t.Fatal("spill corrupted output")
	}
	if err := replay.Close(); err != nil {
		t.Fatal(err)
	}
	if replay.memory.Cap() != 0 {
		t.Fatal("closed replay retained memory")
	}
}
