package proxy

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"strconv"
	"sync"
)

const (
	codexASARHeaderMax  = 8 << 20
	codexASAROutputMax  = 64 << 20
	codexPackageJSONMax = 64 << 10
	codexMSIXBaseURL    = "https://persistent.oaistatic.com/codex-app-prod/"
)

var errCodexMSIXNotFound = errors.New("MSIX not found")

func codexASARPackageLocation(source io.Reader) (int64, int64, int64, error) {
	var prefix [16]byte
	if _, err := io.ReadFull(source, prefix[:]); err != nil {
		return 0, 0, 0, err
	}
	headerSize := int64(binary.LittleEndian.Uint32(prefix[4:8]))
	jsonSize := int64(binary.LittleEndian.Uint32(prefix[12:16]))
	if headerSize < 8 || headerSize > codexASARHeaderMax || jsonSize <= 0 || jsonSize > headerSize-8 {
		return 0, 0, 0, fmt.Errorf("invalid ASAR header size")
	}
	header := make([]byte, jsonSize)
	if _, err := io.ReadFull(source, header); err != nil {
		return 0, 0, 0, err
	}
	var metadata struct {
		Files map[string]struct {
			Offset string `json:"offset"`
			Size   int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(header, &metadata); err != nil {
		return 0, 0, 0, err
	}
	entry, ok := metadata.Files["package.json"]
	if !ok || entry.Size <= 0 || entry.Size > codexPackageJSONMax {
		return 0, 0, 0, fmt.Errorf("ASAR root package.json missing or oversized")
	}
	offset, err := strconv.ParseInt(entry.Offset, 10, 64)
	if err != nil || offset < 0 {
		return 0, 0, 0, fmt.Errorf("invalid ASAR package.json offset")
	}
	target := 8 + headerSize + offset
	consumed := int64(len(prefix)) + jsonSize
	if target < consumed || target+entry.Size > codexASAROutputMax {
		return 0, 0, 0, fmt.Errorf("ASAR package.json exceeds prefix limit")
	}
	return target, entry.Size, consumed, nil
}

func codexASARPackageVersion(source io.Reader) (string, error) {
	target, size, consumed, err := codexASARPackageLocation(source)
	if err != nil {
		return "", err
	}
	if _, err := io.CopyN(io.Discard, source, target-consumed); err != nil {
		return "", err
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(source, data); err != nil {
		return "", err
	}
	return codexDesktopPackageVersion(data)
}

func codexDesktopPackageVersion(data []byte) (string, error) {
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	if _, ok := codexBuildParts(pkg.Version, 3); !ok {
		return "", fmt.Errorf("invalid Desktop build in package.json")
	}
	return pkg.Version, nil
}

// FetchCodexDesktopWindowsBuild 以 Range 分片读取 MSIX 内 ASAR 的压缩前缀。

// 版本化路径并非每个 Store 版本都会上传，缺失时回退到最新包。

// codexWindowsBuildMatchesStore:版本化包必须与清单主次版本一致;回退的最新包常落后于
// 清单几天(Store 先发布、包后上传),只拒绝比清单还新的异常包。
func codexWindowsBuildMatchesStore(build, store []int64, fallback bool) bool {
	if !fallback {
		return build[0] == store[0] && build[1] == store[1]
	}
	return build[0] < store[0] || build[0] == store[0] && build[1] <= store[1]
}

// codexWindowsBuildCache 以 MSIX URL+ETag+大小为键缓存解析结果:包不变时省掉数十次 Range 请求。
var codexWindowsBuildCache codexWindowsBuildMemo

type codexWindowsBuildMemo struct {
	mu      sync.Mutex
	key     string
	version string
}

func (m *codexWindowsBuildMemo) get(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version, m.key == key
}

func (m *codexWindowsBuildMemo) put(key, version string) {
	if key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key, m.version = key, version
}
