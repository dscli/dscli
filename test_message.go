package main

import (
	"encoding/json"
	"fmt"
)

type Message struct {
	Role             string `json:"role"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Content          string `json:"content"`
}

func main() {
	msg := Message{
		Role:             "assistant",
		Content:          "Hello",
		ReasoningContent: "I think the user wants a greeting.",
	}

	data, _ := json.MarshalIndent(msg, "", "  ")
	fmt.Println(string(data))

	// 测试空字符串
	msg.ReasoningContent = ""
	data2, _ := json.MarshalIndent(msg, "", "  ")
	fmt.Println("\n空字符串时：")
	fmt.Println(string(data2))
}
