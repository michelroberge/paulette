package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michelroberge/claudette/backend/internal/model"
)

type ChatRepo struct{}

func NewChatRepo() *ChatRepo {
	return &ChatRepo{}
}

func chatFilePath(hostDir string, stage model.StageName) string {
	return filepath.Join(hostDir, factoryDir, string(stage), "chat-history.json")
}

func (r *ChatRepo) GetHistory(hostDir string, stage model.StageName) ([]model.Message, error) {
	p := chatFilePath(hostDir, stage)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Message{}, nil
		}
		return nil, fmt.Errorf("read chat history: %w", err)
	}
	var history model.ChatHistory
	if err := json.Unmarshal(b, &history); err != nil {
		return nil, fmt.Errorf("parse chat history: %w", err)
	}
	return history.Messages, nil
}

func (r *ChatRepo) AppendMessage(hostDir string, stage model.StageName, msg model.Message) error {
	messages, err := r.GetHistory(hostDir, stage)
	if err != nil {
		return err
	}
	messages = append(messages, msg)

	history := model.ChatHistory{Messages: messages}
	b, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chat history: %w", err)
	}

	p := chatFilePath(hostDir, stage)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("create chat dir: %w", err)
	}
	return os.WriteFile(p, b, 0644)
}
