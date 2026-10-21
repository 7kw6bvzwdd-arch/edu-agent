package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Explicit local real-provider correction using saved evidence, no tenant claims.
func TestDirectAnimationRepairLive(t *testing.T) {
	out := os.Getenv("DIRECT_ANIMATION_REPAIR")
	if out == "" {
		t.Skip("set DIRECT_ANIMATION_REPAIR for real provider correction")
	}
	raw, e := os.ReadFile(out + ".script.json")
	if e != nil {
		t.Fatal(e)
	}
	sc, p := decodeAnimScript(raw)
	if len(p) > 0 {
		t.Fatal(p)
	}
	raw, e = os.ReadFile(out + ".generation.json")
	if e != nil {
		t.Fatal(e)
	}
	var record animGenerationRecord
	if json.Unmarshal(raw, &record) != nil || record.ScriptHash != animDigest(sc) {
		t.Fatal("saved provenance mismatch")
	}
	endpoint, e := chatEndpoint(os.Getenv("ANIMATION_LLM_BASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	key := os.Getenv("ANIMATION_LLM_API_KEY")
	if key == "" || record.Model != os.Getenv("ANIMATION_LLM_MODEL") {
		t.Fatal("model configuration mismatch")
	}
	client := newAnimModelClient()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s := &Server{}
	feedback := "实际语音与停顿共57.8秒，目标60至90秒。请保留已有旁白与视觉源码，增加有具体观察任务的停顿使总时长接近70秒。不要暗加空白，不改成旧组件。"
	if supplied := os.Getenv("DIRECT_ANIMATION_FEEDBACK"); supplied != "" {
		feedback = supplied
	}
	messages := []map[string]string{{"role": "system", "content": animAuthorInstructions(record.Input, "")}, {"role": "user", "content": string(asJSON(sc)) + "\n实际反馈：" + feedback}}
	for attempt := 0; attempt < 3; attempt++ {
		record.Calls++
		response, err := s.postBytesWithClient(ctx, client, endpoint, animModelPayload(record.Model, messages), key)
		if err != nil {
			t.Fatal("model correction failed", err)
		}
		var reply animModelReply
		if json.Unmarshal(response, &reply) != nil || len(reply.Choices) != 1 || reply.Choices[0].FinishReason != "stop" {
			t.Fatal("incomplete model correction")
		}
		record.InputTokens += reply.Usage.Prompt
		record.OutputTokens += reply.Usage.Completion
		content := reply.Choices[0].Message.Content
		sc, p = decodeAnimScript([]byte(content))
		if len(p) == 0 {
			p = animAuthorProblems(sc, record.Input)
		}
		if len(p) == 0 {
			record.ScriptHash = animDigest(sc)
			record.GeneratedAt = time.Now().Format(time.RFC3339)
			if e = os.WriteFile(out+".script.json", asJSON(sc), 0600); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(out+".generation.json", asJSON(record), 0600); e != nil {
				t.Fatal(e)
			}
			e = writeAnimPackage(ctx, sc, out, animVoiceTTS, false, record.Input)
		} else {
			e = fail(422, strings.Join(p, "；"))
		}
		if e == nil {
			return
		}
		messages = append(messages, map[string]string{"role": "assistant", "content": content}, map[string]string{"role": "user", "content": "真实构建失败，请修复源码与时间安排，完整返回JSON：" + e.Error()})
	}
	t.Fatal(e)
}
