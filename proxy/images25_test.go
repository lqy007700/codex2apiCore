package proxy

import (
	"context"
	"fmt"

	"slices"

	"testing"

	"github.com/codex2api/auth"

	"github.com/codex2api/database"

	"github.com/tidwall/gjson"
)

// One handler observes changes made between requests without restarting.

func TestGPTImage25UsageAndResponsesModel(t *testing.T) {
	usage := extractUsageFromResult(gjson.Parse(`{"input_tokens":1550,"input_tokens_details":{"image_tokens":1521},"output_tokens":515,"output_tokens_details":{"image_tokens":515}}`))
	log := buildImageErrorUsageLog(&auth.Account{DBID: 1}, "/v1/images/edits", "gpt-image-2.5-flare", "", false, 0, 0, false, fmt.Errorf("interrupted"), usage, imageUsageLogInfo{})
	if log.ImageInputTokens != 1521 || log.ImageOutputTokens != 515 {
		t.Fatalf("usage detail lost: %+v", log)
	}
	if cost := database.UsageLogBilledCost(log); cost < 0.0277629 || cost > 0.0277631 {
		t.Fatalf("cost = %v", cost)
	}
	body, _ := PrepareResponsesBody([]byte(`{"model":"gpt-5.6-sol","input":"draw","tools":[{"type":"image_generation","model":"gpt-image-2.5-sunburst-2026-09-08","quality":"max"}]}`))
	if gjson.GetBytes(body, "model").String() != "gpt-5.6-sol" || gjson.GetBytes(body, "tools.0.model").String() != "gpt-image-2.5-sunburst-2026-09-08" || gjson.GetBytes(body, "tools.0.quality").String() != "max" {
		t.Fatalf("Responses fields changed: %s", body)
	}
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst-4k"} {
		if !slices.Contains(SupportedModelIDs(context.Background(), nil), model) {
			t.Fatalf("model not exposed: %s", model)
		}
	}
}

func TestGPTImage25OfficialPricingIgnoresBatch(t *testing.T) {
	body := []byte(`Image generation models
Standard
### Grouped Pricing Table data
| Model | Modality | Input | Cached input | Output |
| --- | --- | --- | --- | --- |
| gpt-image-2.5-flare | Image | $8.00 | $2.00 | $30.00 |
| gpt-image-2.5-flare | Text | $5.00 | $1.25 | - |

Batch
### Grouped Pricing Table data
| Model | Modality | Input | Cached input | Output |
| --- | --- | --- | --- | --- |
| gpt-image-2.5-flare | Image | $4.00 | $1.00 | $15.00 |
`)
	prices, err := ParseOpenAIOfficialPricingMarkdown(body)
	if err != nil {
		t.Fatal(err)
	}
	p := prices["gpt-image-2.5-flare"]
	if p.Input != 5 || p.CachedInput != 1.25 || p.ImageInput != 8 || p.CachedImageInput != 2 || p.Output != 30 {
		t.Fatalf("wrong image pricing: %+v", p)
	}
}
