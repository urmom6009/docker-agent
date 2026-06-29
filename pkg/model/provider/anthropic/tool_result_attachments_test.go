package anthropic

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/model/provider/base"
	"github.com/docker/docker-agent/pkg/model/provider/options"
	"github.com/docker/docker-agent/pkg/modelsdev"
)

func TestConvertToolResultBlockUsesClaudeCapabilityFallback(t *testing.T) {
	t.Parallel()
	client := testAttachmentClientWithStore(nil)
	pdf := []byte("%PDF")
	msg := &chat.Message{
		Role:       chat.MessageRoleTool,
		ToolCallID: "toolu_1",
		Content:    "created report",
		MultiContent: []chat.MessagePart{
			{Type: chat.MessagePartTypeText, Text: "created report"},
			{
				Type: chat.MessagePartTypeDocument,
				Document: &chat.Document{
					Name:     "report.pdf",
					MimeType: "application/pdf",
					Source:   chat.DocumentSource{InlineData: pdf},
				},
			},
		},
	}

	block, err := client.convertToolResultBlock(t.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, block.OfToolResult)
	require.Len(t, block.OfToolResult.Content, 2)
	require.NotNil(t, block.OfToolResult.Content[1].OfDocument)
}

func TestConvertToolResultBlockIncludesDocumentAttachments(t *testing.T) {
	t.Parallel()
	client := testAttachmentClient()
	pdf := []byte("%PDF")
	msg := &chat.Message{
		Role:       chat.MessageRoleTool,
		ToolCallID: "toolu_1",
		Content:    "created report",
		MultiContent: []chat.MessagePart{
			{Type: chat.MessagePartTypeText, Text: "created report"},
			{
				Type: chat.MessagePartTypeDocument,
				Document: &chat.Document{
					Name:     "report.pdf",
					MimeType: "application/pdf",
					Source:   chat.DocumentSource{InlineData: pdf},
				},
			},
		},
	}

	block, err := client.convertToolResultBlock(t.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, block.OfToolResult)
	require.Len(t, block.OfToolResult.Content, 2)
	assert.NotNil(t, block.OfToolResult.Content[0].OfText)
	require.NotNil(t, block.OfToolResult.Content[1].OfDocument)
	require.NotNil(t, block.OfToolResult.Content[1].OfDocument.Source.OfBase64)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pdf), block.OfToolResult.Content[1].OfDocument.Source.OfBase64.Data)
}

func TestConvertToolResultBlockNeverSendsEmptyContent(t *testing.T) {
	t.Parallel()
	client := testAttachmentClient()
	msg := &chat.Message{
		Role:       chat.MessageRoleTool,
		ToolCallID: "toolu_1",
		Content:    "   ",
	}

	block, err := client.convertToolResultBlock(t.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, block.OfToolResult)
	require.Len(t, block.OfToolResult.Content, 1)
	require.NotNil(t, block.OfToolResult.Content[0].OfText)
	assert.Equal(t, "(no output)", block.OfToolResult.Content[0].OfText.Text)
}

func TestConvertToolResultBlockIncludesImageDocumentAttachments(t *testing.T) {
	t.Parallel()
	client := testAttachmentClient()
	image := []byte("PNG")
	msg := &chat.Message{
		Role:       chat.MessageRoleTool,
		ToolCallID: "toolu_1",
		Content:    "screenshot",
		MultiContent: []chat.MessagePart{
			{Type: chat.MessagePartTypeText, Text: "screenshot"},
			{
				Type: chat.MessagePartTypeDocument,
				Document: &chat.Document{
					Name:     "screenshot.png",
					MimeType: "image/png",
					Source:   chat.DocumentSource{InlineData: image},
				},
			},
		},
	}

	block, err := client.convertToolResultBlock(t.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, block.OfToolResult)
	require.Len(t, block.OfToolResult.Content, 2)
	require.NotNil(t, block.OfToolResult.Content[1].OfImage)
	require.NotNil(t, block.OfToolResult.Content[1].OfImage.Source.OfBase64)
	assert.Equal(t, base64.StdEncoding.EncodeToString(image), block.OfToolResult.Content[1].OfImage.Source.OfBase64.Data)
	assert.Equal(t, "image/png", string(block.OfToolResult.Content[1].OfImage.Source.OfBase64.MediaType))
}

func testAttachmentClient() *Client {
	store := modelsdev.NewDatabaseStore(&modelsdev.Database{
		Providers: map[string]modelsdev.Provider{
			"anthropic": {
				Models: map[string]modelsdev.Model{
					"claude-test": {
						Modalities: modelsdev.Modalities{Input: []string{"text", "image", "pdf"}},
					},
				},
			},
		},
	})

	return testAttachmentClientWithStore(store)
}

func testAttachmentClientWithStore(store *modelsdev.Store) *Client {
	var modelOptions options.ModelOptions
	if store != nil {
		options.WithModelsDevStore(store)(&modelOptions)
	}

	return &Client{Config: base.Config{
		ModelConfig: latest.ModelConfig{
			Provider: "anthropic",
			Model:    "claude-test",
		},
		ModelOptions: modelOptions,
	}}
}
