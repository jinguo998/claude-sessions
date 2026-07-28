package opencode

import "encoding/json"

const metadataVersion = "opencode-sqlite-v1"

type sessionRow struct {
	ID             string `json:"id"`
	Directory      string `json:"directory"`
	Title          string `json:"title"`
	Agent          string `json:"agent"`
	Model          string `json:"model"`
	TokensInput    int    `json:"tokens_input"`
	TokensOutput   int    `json:"tokens_output"`
	TimeCreated    int64  `json:"time_created"`
	TimeUpdated    int64  `json:"time_updated"`
	MessageUpdated int64  `json:"message_updated"`
	PartUpdated    int64  `json:"part_updated"`
	MessageCount   int    `json:"message_count"`
	PartCount      int    `json:"part_count"`
	UserCount      int    `json:"user_count"`
	ToolCount      int    `json:"tool_count"`
}

type userTextRow struct {
	TimeCreated int64  `json:"time_created"`
	Text        string `json:"text"`
}

type previewRow struct {
	MessageID   string          `json:"message_id"`
	Role        string          `json:"role"`
	MessageTime int64           `json:"message_time"`
	PartTime    int64           `json:"part_time"`
	PartData    json.RawMessage `json:"part_data"`
}

type partData struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Tool     string          `json:"tool"`
	State    json.RawMessage `json:"state"`
	Filename string          `json:"filename"`
	URL      string          `json:"url"`
	Files    json.RawMessage `json:"files"`
}

type toolState struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

type modelData struct {
	ID         string `json:"id"`
	ModelID    string `json:"modelID"`
	ProviderID string `json:"providerID"`
}
