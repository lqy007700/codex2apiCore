package admin

import (
	"context"

	"errors"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"os"

	"strings"
	"time"

	"github.com/codex2api/database"

	"github.com/codex2api/internal/imagestore"
)

const defaultImageAssetDir = "/data/images"
const maxInlineImageAssetCacheBytes = 64 * 1024 * 1024
const defaultSignedImageThumbKB = 32
const maxImageJobOutputCount = 4

var errInvalidUpscaledImage = errors.New("invalid upscaled image")

// thumbCache 跨请求复用缩略图。S3 后端读源图代价高，这里用 LRU 兜一下。
var thumbCache = imagestore.NewThumbnailCache(0)

type imagePromptTemplatePayload struct {
	Name         *string  `json:"name"`
	Prompt       *string  `json:"prompt"`
	Model        *string  `json:"model"`
	Size         *string  `json:"size"`
	Quality      *string  `json:"quality"`
	OutputFormat *string  `json:"output_format"`
	Background   *string  `json:"background"`
	Style        *string  `json:"style"`
	Tags         []string `json:"tags"`
	Favorite     *bool    `json:"favorite"`
}

type imageGenerationJobPayload struct {
	StorageMode      string   `json:"storage_mode,omitempty"`
	RetentionSeconds int64    `json:"retention_seconds,omitempty"`
	Prompt           string   `json:"prompt"`
	Model            string   `json:"model"`
	Size             string   `json:"size"`
	Quality          string   `json:"quality"`
	OutputFormat     string   `json:"output_format"`
	Background       string   `json:"background"`
	Style            string   `json:"style"`
	Upscale          string   `json:"upscale"`
	StrictSize       *bool    `json:"strict_size,omitempty"`
	UpscaleFit       string   `json:"upscale_fit,omitempty"`
	N                int      `json:"n"`
	APIKeyID         int64    `json:"api_key_id"`
	TemplateID       int64    `json:"template_id"`
	InputImages      []string `json:"input_images"`
	External         bool     `json:"-"`
}

type imageJobResponse struct {
	Job *database.ImageGenerationJob `json:"job"`
}

type externalImageJobResponse struct {
	Job *database.ImageGenerationJob `json:"job"`
}

type imageAssetFileOptions struct {
	download        bool
	private         bool
	thumbKB         int
	requireAssetDir bool
}

type imagePromptTemplatesResponse struct {
	Templates []database.ImagePromptTemplate `json:"templates"`
}

// 本地后端走 Stat 快速判断尺寸；S3 后端没有便宜的 Stat，直接尝试 Read 并按字节数过滤。

// Preserve the original endpoint's full-image fallback for formats that
// cannot be thumbnailed. The source is streamed without pixel decoding.

// S3 偶尔不带 Content-Length，退化为 chunked。

// S3 ref 不在本地目录约束内，由后端自身保证作用域。

// imageJobRunOptions carries admission state from the enqueue site into the
// background job.
type imageJobRunOptions struct {
	// sharedAPIKeyConcurrency reports that the enqueue path already reserved the
	// API key's concurrency slot for this whole job, so the in-process upstream
	// calls must not reserve a second one for the same key.
	sharedAPIKeyConcurrency bool
	queueContext            context.Context
	queueOwner              string
}

// imageJobBaseTimeout budgets one upstream output. A batch issues one call per
// output, and may repeat the whole batch once through the JPEG fallback, so the
// deadline scales with the requested count instead of capping every job at the
// single-output budget.
const imageJobBaseTimeout = 12 * time.Minute

// imageJobStatusContext backs the terminal status write. The job's own context
// may already be past its deadline by the time the job finishes, and reusing it
// would leave the row stuck in "running" until the next restart.

// Outputs saved before the failure remain available and will be billed.
// Report the failure as a warning so they stay reachable instead of
// becoming orphaned files behind a failed job.

// upstreamImageJobSize converts an exact output canvas into a size accepted by
// GPT Image 2. The upstream image tool requires both dimensions to be multiples
// of 16, while callers commonly request display sizes such as 1920x1080. The
// original req.Size remains unchanged and is applied after generation, so this
// compatibility adjustment never changes the promised final dimensions.

// LocalBackend 已在 Configure 时 mkdir，这里再保险一次以兼容 dir 在运行期被外部清理的情况。

// Undecodable upscaler output is degraded the same way a
// transport failure is. The upstream image behind it has
// already been generated and billed, so keep it and warn
// rather than failing every output in the batch.

func imageAssetDir() string {
	if dir := strings.TrimSpace(os.Getenv("IMAGE_ASSET_DIR")); dir != "" {
		return dir
	}
	return defaultImageAssetDir
}

// normalizeImageStudioModel 生图台只接受 Codex 生图模型族(gpt-image-2 及 -2k/-4k 档位
// 别名、带日期快照名);其他名字回落到 gpt-image-2。
// Flare/Sunburst、日期快照和尺寸别名均按 gpt-image-* 前缀准入。

// An absent value means "decide from the model and size"; "none" is the
// explicit opt-out that suppresses that inference, so a caller can take the
// upstream image as-is even when it asked for a size upstream cannot render.

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
