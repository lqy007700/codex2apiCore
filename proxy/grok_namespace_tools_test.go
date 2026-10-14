package proxy

import (
	"encoding/json"

	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestGrokCustomResponseRestoration(t *testing.T) {
	aliases := map[string]grokNsIdentity{"apply_patch": {Name: "apply_patch", Custom: true}}
	nonStream := reverseGrokNamespaceJSON([]byte(`{"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"patch text\"}"}]}`), aliases)
	if got := gjson.GetBytes(nonStream, "output.0.type").String(); got != "custom_tool_call" {
		t.Fatalf("non-stream type = %q; body=%s", got, nonStream)
	}
	if got := gjson.GetBytes(nonStream, "output.0.id").String(); got != "ctc_1" {
		t.Fatalf("non-stream custom id = %q, want ctc_1; body=%s", got, nonStream)
	}
	if got := gjson.GetBytes(nonStream, "output.0.input").String(); got != "patch text" {
		t.Fatalf("non-stream input = %q; body=%s", got, nonStream)
	}

	r := &grokStreamReverser{aliases: aliases, customItems: map[string]bool{}, toolSearchItems: map[string]bool{}, inputBytes: map[string]int{}}
	added := r.rewriteLine([]byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"apply_patch\",\"arguments\":\"\"}}\n"))
	if got := gjson.GetBytes(bytesAfterSSEData(added), "item.type").String(); got != "custom_tool_call" {
		t.Fatalf("stream added type = %q; line=%s", got, added)
	}
	if got := gjson.GetBytes(bytesAfterSSEData(added), "item.id").String(); got != "ctc_1" {
		t.Fatalf("stream added custom id = %q, want ctc_1; line=%s", got, added)
	}
	delta := r.rewriteLine([]byte("data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\"}\n"))
	if delta != nil {
		t.Fatalf("custom function delta leaked: %s", delta)
	}
	done := r.rewriteLine([]byte("data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_1\",\"arguments\":\"{\\\"input\\\":\\\"patch text\\\"}\"}\n"))
	if got := gjson.GetBytes(bytesAfterSSEData(done), "type").String(); got != "response.custom_tool_call_input.done" {
		t.Fatalf("stream done type = %q; line=%s", got, done)
	}
	if got := gjson.GetBytes(bytesAfterSSEData(done), "input").String(); got != "patch text" {
		t.Fatalf("stream done input = %q; line=%s", got, done)
	}
	if got := gjson.GetBytes(bytesAfterSSEData(done), "item_id").String(); got != "ctc_1" {
		t.Fatalf("stream done custom item_id = %q, want ctc_1; line=%s", got, done)
	}
	itemDone := r.rewriteLine([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"apply_patch\",\"arguments\":\"{\\\"input\\\":\\\"patch text\\\"}\"}}\n"))
	if got := gjson.GetBytes(bytesAfterSSEData(itemDone), "item.id").String(); got != "ctc_1" {
		t.Fatalf("stream done item custom id = %q, want ctc_1; line=%s", got, itemDone)
	}
	completed := r.rewriteLine([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"apply_patch\",\"arguments\":\"{\\\"input\\\":\\\"patch text\\\"}\"}]}}\n"))
	if got := gjson.GetBytes(bytesAfterSSEData(completed), "response.output.0.id").String(); got != "ctc_1" {
		t.Fatalf("stream completed custom id = %q, want ctc_1; line=%s", got, completed)
	}
}

// 习惯名反解映射,不是出站声明名(map 迭代顺序随机,不跳过会间歇取错)

func TestRetypeGrokResponsesToolCallItemIDOnlyChangesKnownPrefixes(t *testing.T) {
	tests := []struct {
		id       string
		itemType string
		want     string
	}{
		{id: "fc_custom", itemType: "custom_tool_call", want: "ctc_custom"},
		{id: "fc_search", itemType: "tool_search_call", want: "tsc_search"},
		{id: "ctc_custom", itemType: "custom_tool_call", want: "ctc_custom"},
		{id: "external-id", itemType: "custom_tool_call", want: "external-id"},
		{id: "fc_function", itemType: "function_call", want: "fc_function"},
	}
	for _, tc := range tests {
		if got := retypeGrokResponsesToolCallItemID(tc.id, tc.itemType); got != tc.want {
			t.Errorf("retype(%q, %q) = %q, want %q", tc.id, tc.itemType, got, tc.want)
		}
	}
}

func grokGiantCustomToolFailure(t *testing.T, r *grokStreamReverser) []byte {
	t.Helper()
	large := strings.Repeat("x", grokToolCallHardLimitBytes+1)
	line := []byte("data: " + string(mustJSON(t, map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_big", "delta": large})) + "\n")
	failure := r.rewriteLine(line)
	if got := gjson.GetBytes(bytesAfterSSEData(failure), "type").String(); got != "response.failed" {
		t.Fatalf("oversized custom call did not fail: type=%q line=%s", got, failure)
	}
	if got := gjson.GetBytes(bytesAfterSSEData(failure), "response.error.code").String(); got != "invalid_prompt" {
		t.Fatalf("oversized custom error code = %q", got)
	}
	return failure
}

// Fix the constructor fallback to a sentinel so this test cannot accidentally
// pass when failureLine samples the wall clock in the same second.

func TestGrokGiantToolGuardIsCaseInsensitiveAndPreservesNumbers(t *testing.T) {
	body := []byte(`{"seed":9007199254740993,"tools":[{"type":"custom","name":"APPLY_PATCH"}],"input":"patch"}`)
	guarded, changed := addGrokGiantToolInstructions(body)
	if !changed {
		t.Fatal("case-insensitive apply_patch name did not enable guard")
	}
	if !strings.Contains(string(guarded), `"seed":9007199254740993`) {
		t.Fatalf("large integer changed during guard injection: %s", guarded)
	}
}

func TestGrokAdditionalToolsPreservesNumbers(t *testing.T) {
	body := []byte(`{"seed":9007199254740993,"input":[{"type":"additional_tools","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}]}`)
	lifted, changed := liftGrokAdditionalTools(body)
	if !changed {
		t.Fatal("additional_tools carrier was not lifted")
	}
	if !strings.Contains(string(lifted), `"seed":9007199254740993`) {
		t.Fatalf("large integer changed while lifting tools: %s", lifted)
	}
}

func TestGrokPlainFunctionDoneBypassesBridgedSizeLimit(t *testing.T) {
	r := &grokStreamReverser{aliases: map[string]grokNsIdentity{}, customItems: map[string]bool{}, toolSearchItems: map[string]bool{}, inputBytes: map[string]int{}}
	arguments := strings.Repeat("x", grokToolCallHardLimitBytes+1)
	line := []byte("data: " + string(mustJSON(t, map[string]any{"type": "response.function_call_arguments.done", "item_id": "plain", "arguments": arguments})) + "\n")
	got := r.rewriteLine(line)
	if string(got) != string(line) {
		t.Fatalf("plain function call was rewritten or rejected: %s", got)
	}
}

func TestGrokStreamFastPathIncludesFunctionArgumentsEvents(t *testing.T) {
	r := &grokStreamReverser{aliases: map[string]grokNsIdentity{}, customItems: map[string]bool{"custom": true}, toolSearchItems: map[string]bool{}, inputBytes: map[string]int{}}
	line := []byte("data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"custom\",\"delta\":\"{}\"}\n")
	if got := r.rewriteLine(line); got != nil {
		t.Fatalf("bridged arguments delta bypassed stream handling: %s", got)
	}
	if r.inputBytes["custom"] != 2 {
		t.Fatalf("bridged arguments delta was not accounted: %d", r.inputBytes["custom"])
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func bytesAfterSSEData(line []byte) []byte {
	trimmed := strings.TrimSpace(string(line))
	return []byte(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
}

// namespace 工具应被展平：没有 type:"namespace" 残留，子函数升到顶层且改名。

// tool_choice 改写为扁平名、去掉 namespace。

// input 历史 function_call 也改写为扁平名。

// Codex 的 web_search 带 external_web_access 等控制字段 → 上游 400；应剥离为最小形态。

// allowed_domains 约束保留在 filters 内。

// external_web_access:false 无法在上游表达，整体移除工具并撤掉指向它的 tool_choice。

// preview 变体应归一为 web_search（Grok 不认 web_search_preview）。

func TestStripGrokReasoningEncryptedContent(t *testing.T) {
	// 真实形态：reasoning 项带 encrypted_content 外来密文；应删掉密文、保留明文 summary。
	body := []byte(`{"model":"grok-4.5","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}],"content":null,"encrypted_content":"PefVPtNeWwpXfOREIGN"}` +
		`]}`)
	out := stripGrokUndecodableBlobs(body)
	if gjson.GetBytes(out, "input.1.encrypted_content").Exists() {
		t.Fatalf("encrypted_content not stripped: %s", string(out))
	}
	if got := gjson.GetBytes(out, "input.1.type").String(); got != "reasoning" {
		t.Fatalf("reasoning item type changed to %q", got)
	}
	if got := gjson.GetBytes(out, "input.1.summary.0.text").String(); got != "thinking" {
		t.Fatalf("plaintext summary lost: %s", string(out))
	}
	// content:null 必须被删除，否则去密文后的 reasoning 变体不匹配（422）。
	if gjson.GetBytes(out, "input.1.content").Exists() {
		t.Fatalf("null content not dropped: %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.content.0.text").String() != "hi" {
		t.Fatal("retained message altered")
	}
}

func TestStripGrokEmptyReasoningBecomesBoundary(t *testing.T) {
	// reasoning 项去掉密文后无明文可回放 → 替换成 developer 边界消息。
	body := []byte(`{"model":"grok-4.5","input":[{"type":"reasoning","summary":[],"content":null,"encrypted_content":"FOREIGN"}]}`)
	out := stripGrokUndecodableBlobs(body)
	if gjson.GetBytes(out, "input.0.type").String() != "message" || gjson.GetBytes(out, "input.0.role").String() != "developer" {
		t.Fatalf("empty reasoning not replaced with boundary: %s", string(out))
	}
}

func TestStripGrokCompactionItem(t *testing.T) {
	// remote_compaction_v2 的 type:"compaction" 项整体替换成边界消息。
	body := []byte(`{"model":"grok-4.5","input":[{"type":"compaction","encrypted_content":"OPAQUE"}]}`)
	out := stripGrokUndecodableBlobs(body)
	if gjson.GetBytes(out, `input.#(type=="compaction")`).Exists() {
		t.Fatalf("compaction item not stripped: %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.role").String() != "developer" {
		t.Fatalf("compaction not replaced: %s", string(out))
	}
}

func TestStripGrokUndecodableBlobsNoop(t *testing.T) {
	body := []byte(`{"model":"grok-4.5","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	if string(stripGrokUndecodableBlobs(body)) != string(body) {
		t.Fatal("body without encrypted content should be unchanged")
	}
}

func TestReverseGrokNamespaceJSON(t *testing.T) {
	aliases := map[string]grokNsIdentity{
		"mcp__calendar__list": {Namespace: "mcp__calendar__", Name: "list"},
	}
	// 完整响应里的 function_call 应被反解回 {name, namespace}。
	data := []byte(`{"output":[{"type":"function_call","call_id":"c1","name":"mcp__calendar__list","arguments":"{}"}]}`)
	out := reverseGrokNamespaceJSON(data, aliases)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	call := parsed["output"].([]any)[0].(map[string]any)
	if call["name"] != "list" || call["namespace"] != "mcp__calendar__" {
		t.Fatalf("function_call not restored: %v", call)
	}
}

// 非 function_call 行原样返回。

func TestRebuildGrokHistoryToNativeContract(t *testing.T) {
	reg := func(ns, name string, _, _ bool) string { return ns + "__" + name }
	// Codex 发的历史项带扩展字段/null；重建后应只剩 Grok 原生字段。
	cases := []struct {
		name   string
		in     string
		typ    string
		reject []string // 重建后不应出现的字段
		keep   map[string]string
	}{
		{
			name:   "function_call drops id/status/metadata",
			in:     `{"type":"function_call","id":"fc_1","status":"completed","call_id":"c1","name":"run","arguments":"{}","internal_chat_message_metadata_passthrough":{"turn_id":"t1"}}`,
			typ:    "function_call",
			reject: []string{"id", "status", "internal_chat_message_metadata_passthrough"},
			keep:   map[string]string{"call_id": "c1", "name": "run"},
		},
		{
			name:   "reasoning drops null content, keeps summary+encrypted",
			in:     `{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"t"}],"content":null,"encrypted_content":"BLOB","status":"completed"}`,
			typ:    "reasoning",
			reject: []string{"content", "status"},
			keep:   map[string]string{"id": "rs_1", "encrypted_content": "BLOB"},
		},
		{
			name:   "function_call_output keeps only call_id+output",
			in:     `{"type":"function_call_output","id":"fco_1","call_id":"c1","output":"done","status":"completed"}`,
			typ:    "function_call_output",
			reject: []string{"id", "status"},
			keep:   map[string]string{"call_id": "c1", "output": "done"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var item map[string]any
			_ = json.Unmarshal([]byte(tc.in), &item)
			rebuilt, changed := rebuildGrokHistoryItem(item, reg)
			if !changed {
				t.Fatalf("expected rebuild")
			}
			out, _ := json.Marshal(rebuilt)
			for _, f := range tc.reject {
				if gjson.GetBytes(out, f).Exists() {
					t.Fatalf("field %q should be dropped: %s", f, out)
				}
			}
			for k, want := range tc.keep {
				if got := gjson.GetBytes(out, k).String(); got != want {
					t.Fatalf("field %q = %q, want %q", k, got, want)
				}
			}
			if gjson.GetBytes(out, "type").String() != tc.typ {
				t.Fatalf("type lost")
			}
		})
	}
}

// 端到端：多轮 body（无 namespace/web_search，仅历史项)也应触发重建。

// Grok 上游要求函数工具参数 schema 根节点必须是单一 object(联合根会 400
// "tool parameter root must be an object type")。桥接层必须把联合根合并、
// 非 object 根降级,且不得改动本就合规的 schema 与嵌套联合。

// 联合中含被丢弃的 null 分支(可空调用),分支侧 required 不得并入。

// 合规的纯 object schema 不应被改写(改写会破坏上游缓存前缀稳定性)。

// 真实事故形态(2026-08-26 用户回报):schema 根同时带 type:"object" 与 anyOf,
// Grok 仍按联合根拒绝。归一必须以"根上出现 anyOf/oneOf/allOf"为准,不能因为
// type 已是 object 就放行;根自身的 properties/required 要并进合并结果。

// 联合根合并必须保留根级非联合 sibling 键:分支属性里的 "$ref":"#/$defs/X" 指向
// 根级 $defs,丢掉定义容器会产生悬空引用,上游拿到不可解析的 schema 照样 400。
// additionalProperties:false 等对象约束同理不得静默放宽(PR #580 评审)。

// 判别联合(discriminated union):多个分支对同一属性给出不同 schema(如
// const:"create" 与 const:"update")时不能先到先得——那会让后续分支的合法调用
// 被上游拒绝。冲突属性必须包成嵌套 anyOf,把每个分支的定义都保住。

// 两个分支都必填 kind 且没有非 object 分支被丢弃,交集应保留 kind。

// 分支定义完全相同的属性不包联合,保持单值形态。

// allOf 根的同名属性冲突包成嵌套 allOf:全部分支约束同时成立,不能丢弃任何一侧。

// 纯 $ref 联合分支(Pydantic/Zod 常见形态)必须先解析到根级 $defs 再合并,
// 不能当作非 object 分支丢弃——那会丢掉整个对象定义。

// null 分支被丢弃 → 分支侧 required 放宽,和内联 object 分支的行为一致。

// JSON Schema 布尔形态(parameters:true/false)不能原样透传——上游同样按
// "root must be an object type" 拒绝。true 降级为宽松 object,false 给空封闭 object。

// type:["object","null"] 与 anyOf:[object,{type:"null"}] 语义等价,required 的
// 处理必须一致:null 选项折叠丢弃后放弃 required 强制,保住原本合法的空参调用。

// Grok 上游把 "tool_search" 保留给内置工具,同名 function 声明会被
// 400 拒绝。代理别名与用户自带的同名 function 都必须避开保留名,
// 且模型按声明名或习惯名回调时都能反解。

// 历史 function_call 的保留名必须跟声明侧一起改名:声明被挪到 tool_search_fn
// 而历史仍引用 tool_search 时,上游按保留名/未声明名拒绝,或与内置 tool_search
// 混淆。多轮工具会话是常态形态,声明与历史必须始终同名。

// 本轮未再声明该工具(如工具被裁剪)时,历史里的保留名同样不能原样出站,
// 且纯历史改名必须触发重新序列化。
