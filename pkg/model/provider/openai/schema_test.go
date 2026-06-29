package openai

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/tools"
)

func TestMakeAllRequired(t *testing.T) {
	t.Parallel()
	type DirectoryTreeArgs struct {
		Path     string `json:"path" jsonschema:"The directory path to traverse"`
		MaxDepth int    `json:"max_depth,omitempty" jsonschema:"Maximum depth to traverse (optional)"`
	}
	schema := tools.MustSchemaFor[DirectoryTreeArgs]()

	schemaMap, err := tools.SchemaToMap(schema)
	require.NoError(t, err)
	required := schemaMap["required"].([]any)
	assert.Len(t, required, 1)
	assert.Contains(t, required, "path")

	updatedSchema := makeAllRequired(schemaMap)
	required = updatedSchema["required"].([]any)
	assert.Len(t, required, 2)
	assert.Contains(t, required, "max_depth")
	assert.Contains(t, required, "path")
}

func TestMakeAllRequired_NoParameter(t *testing.T) {
	t.Parallel()
	type NoArgs struct{}
	schema := tools.MustSchemaFor[NoArgs]()

	schemaMap, err := tools.SchemaToMap(schema)
	require.NoError(t, err)

	buf, err := json.Marshal(schemaMap)
	require.NoError(t, err)
	assert.JSONEq(t, `{"additionalProperties":false,"properties":{},"type":"object"}`, string(buf))

	updatedSchema := makeAllRequired(schemaMap)
	buf, err = json.Marshal(updatedSchema)
	require.NoError(t, err)
	assert.JSONEq(t, `{"additionalProperties":false,"properties":{},"type":"object","required":[]}`, string(buf))
}

func TestMakeAllRequired_NilSchema(t *testing.T) {
	t.Parallel()
	updatedSchema := makeAllRequired(nil)
	buf, err := json.Marshal(updatedSchema)
	require.NoError(t, err)
	assert.JSONEq(t, `{"additionalProperties":false,"properties":{},"type":"object","required":[]}`, string(buf))
}

func TestMakeAllRequired_AnyOf(t *testing.T) {
	t.Parallel()
	// Reproduces the chrome-devtools-mcp "emulate" tool schema where
	// viewport has an anyOf with an object variant whose properties
	// are not all listed in required. OpenAI rejects this.
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"viewport": map[string]any{
				"anyOf": []any{
					map[string]any{
						"type": "object",
						"properties": map[string]any{
							"width":             map[string]any{"type": "number"},
							"height":            map[string]any{"type": "number"},
							"deviceScaleFactor": map[string]any{"type": "number"},
						},
						"required": []any{"width", "height"},
					},
					map[string]any{
						"type": "null",
					},
				},
			},
		},
		"required": []any{"viewport"},
	}

	updated := makeAllRequired(schema)

	// Top-level: viewport must be required
	required := updated["required"].([]any)
	assert.Contains(t, required, "viewport")

	// anyOf[0]: all properties must be required, including deviceScaleFactor
	viewport := updated["properties"].(map[string]any)["viewport"].(map[string]any)
	anyOf := viewport["anyOf"].([]any)
	variant := anyOf[0].(map[string]any)
	variantRequired := variant["required"].([]any)
	assert.Len(t, variantRequired, 3)
	assert.Contains(t, variantRequired, "width")
	assert.Contains(t, variantRequired, "height")
	assert.Contains(t, variantRequired, "deviceScaleFactor")

	// deviceScaleFactor was not originally required, so its type should be nullable
	dsf := variant["properties"].(map[string]any)["deviceScaleFactor"].(map[string]any)
	assert.Equal(t, []string{"number", "null"}, dsf["type"])

	// width was originally required, so its type should be unchanged
	w := variant["properties"].(map[string]any)["width"].(map[string]any)
	assert.Equal(t, "number", w["type"])
}

func TestMakeAllRequired_NestedProperties(t *testing.T) {
	t.Parallel()
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string"},
					"value": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		},
		"required": []any{"config"},
	}

	updated := makeAllRequired(schema)

	// Nested object: all properties must be required
	config := updated["properties"].(map[string]any)["config"].(map[string]any)
	configRequired := config["required"].([]any)
	assert.Len(t, configRequired, 2)
	assert.Contains(t, configRequired, "name")
	assert.Contains(t, configRequired, "value")

	// value was not originally required, so its type should be nullable
	value := config["properties"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, []string{"string", "null"}, value["type"])
}

func TestMakeAllRequired_ArrayItems(t *testing.T) {
	t.Parallel()
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":   map[string]any{"type": "string"},
						"name": map[string]any{"type": "string"},
					},
					"required": []any{"id"},
				},
			},
		},
		"required": []any{"items"},
	}

	updated := makeAllRequired(schema)

	// Array items object: all properties must be required
	itemsSchema := updated["properties"].(map[string]any)["items"].(map[string]any)
	itemObj := itemsSchema["items"].(map[string]any)
	itemRequired := itemObj["required"].([]any)
	assert.Len(t, itemRequired, 2)
	assert.Contains(t, itemRequired, "id")
	assert.Contains(t, itemRequired, "name")
}

func TestMakeAllRequired_AdditionalProperties(t *testing.T) {
	t.Parallel()
	// Schema-form additionalProperties (Notion-style) must be preserved so the
	// model knows what dictionary values look like. The inner schema is still
	// normalized: all its properties become required, newly-required ones are
	// nullable, and inner object nodes get additionalProperties: false.
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"children": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"bulleted_list_item": map[string]any{"type": "string"},
						"numbered_list_item": map[string]any{"type": "string"},
					},
					"required": []any{"bulleted_list_item"},
				},
			},
		},
		"required": []any{"children"},
	}

	updated := makeAllRequired(schema)

	// Outer object: no additionalProperties was set, so it is forced to false.
	assert.Equal(t, false, updated["additionalProperties"])

	// `children` declares schema-form additionalProperties — left as-is.
	children := updated["properties"].(map[string]any)["children"].(map[string]any)
	additionalProps, ok := children["additionalProperties"].(map[string]any)
	require.True(t, ok, "schema-form additionalProperties must be preserved")

	// The inner schema is still normalized.
	additionalRequired := additionalProps["required"].([]any)
	assert.Len(t, additionalRequired, 2)
	assert.Contains(t, additionalRequired, "bulleted_list_item")
	assert.Contains(t, additionalRequired, "numbered_list_item")

	numberedListItem := additionalProps["properties"].(map[string]any)["numbered_list_item"].(map[string]any)
	assert.Equal(t, []string{"string", "null"}, numberedListItem["type"])

	bulletedListItem := additionalProps["properties"].(map[string]any)["bulleted_list_item"].(map[string]any)
	assert.Equal(t, "string", bulletedListItem["type"])
}

func TestConvertParametersToSchema_JiraEditIssueFields(t *testing.T) {
	t.Parallel()
	// Regression for the Atlassian remote MCP `editJiraIssue` tool, whose
	// `fields` property declared additionalProperties as an object schema.
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"issueIdOrKey": map[string]any{"type": "string"},
			"fields": map[string]any{
				"type":                 "object",
				"description":          "Jira issue fields to update",
				"additionalProperties": map[string]any{},
			},
		},
		"required": []any{"issueIdOrKey", "fields"},
	}

	result, strict, err := ConvertParametersToSchema(schema)
	require.NoError(t, err)

	// Schema-form additionalProperties => non-strict, schema preserved as-is.
	assert.False(t, strict)
	fields := result["properties"].(map[string]any)["fields"].(map[string]any)
	_, isMap := fields["additionalProperties"].(map[string]any)
	assert.True(t, isMap, "non-strict path must preserve schema-form additionalProperties")
}

func TestRemoveFormatFields(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"format":      "uri",
				"description": "The URL",
			},
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "The name",
			},
		},
	}

	updated := removeFormatFields(schema)

	url := updated["properties"].(map[string]any)["url"].(map[string]any)
	assert.Equal(t, "string", url["type"])
	assert.Equal(t, "The URL", url["description"])
	assert.NotContains(t, url, "format")

	email := updated["properties"].(map[string]any)["email"].(map[string]any)
	assert.Equal(t, "string", email["type"])
	assert.NotContains(t, email, "format")

	name := updated["properties"].(map[string]any)["name"].(map[string]any)
	assert.Equal(t, "string", name["type"])
	assert.Equal(t, "The name", name["description"])
}

func TestRemoveFormatFields_NestedObjects(t *testing.T) {
	t.Parallel()
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"user": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{
						"type":   "string",
						"format": "email",
					},
					"website": map[string]any{
						"type":   "string",
						"format": "uri",
					},
				},
			},
			"name": map[string]any{
				"type":   "string",
				"format": "hostname",
			},
		},
	}

	updated := removeFormatFields(schema)

	user := updated["properties"].(map[string]any)["user"].(map[string]any)
	email := user["properties"].(map[string]any)["email"].(map[string]any)
	assert.NotContains(t, email, "format")
	assert.Equal(t, "string", email["type"])

	website := user["properties"].(map[string]any)["website"].(map[string]any)
	assert.NotContains(t, website, "format")

	name := updated["properties"].(map[string]any)["name"].(map[string]any)
	assert.NotContains(t, name, "format")
}

func TestRemoveFormatFields_ArrayItems(t *testing.T) {
	t.Parallel()
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"urls": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":   "string",
					"format": "uri",
				},
			},
			"contacts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email": map[string]any{
							"type":   "string",
							"format": "email",
						},
					},
				},
			},
		},
	}

	updated := removeFormatFields(schema)

	urls := updated["properties"].(map[string]any)["urls"].(map[string]any)
	urlItems := urls["items"].(map[string]any)
	assert.NotContains(t, urlItems, "format")
	assert.Equal(t, "string", urlItems["type"])

	contacts := updated["properties"].(map[string]any)["contacts"].(map[string]any)
	contactItems := contacts["items"].(map[string]any)
	email := contactItems["properties"].(map[string]any)["email"].(map[string]any)
	assert.NotContains(t, email, "format")
	assert.Equal(t, "string", email["type"])
}

func TestRemoveFormatFields_NilSchema(t *testing.T) {
	t.Parallel()
	assert.Nil(t, removeFormatFields(nil))
}

func TestRemoveFormatFields_NoProperties(t *testing.T) {
	t.Parallel()
	schema := shared.FunctionParameters{"type": "object"}
	updated := removeFormatFields(schema)
	assert.Equal(t, schema, updated)
}

func TestMakeAllRequired_TypeArrayWithObject(t *testing.T) {
	t.Parallel()
	// Reproduces the user_prompt tool schema where a property has
	// type: ["object", "null"] with nested properties. OpenAI requires
	// these nested properties to also have additionalProperties: false.
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"schema": map[string]any{
				"type": []string{"object", "null"},
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"age":  map[string]any{"type": "number"},
				},
				"required": []any{"name"},
			},
		},
		"required": []any{"schema"},
	}

	updated := makeAllRequired(schema)

	// Top-level should have additionalProperties: false
	assert.Equal(t, false, updated["additionalProperties"])

	// The schema property should also have additionalProperties: false
	schemaProps := updated["properties"].(map[string]any)["schema"].(map[string]any)
	assert.Equal(t, false, schemaProps["additionalProperties"])

	// All properties in schema should be required
	schemaRequired := schemaProps["required"].([]any)
	assert.Len(t, schemaRequired, 2)
	assert.Contains(t, schemaRequired, "name")
	assert.Contains(t, schemaRequired, "age")

	// age was not originally required, so its type should be nullable
	age := schemaProps["properties"].(map[string]any)["age"].(map[string]any)
	assert.Equal(t, []string{"number", "null"}, age["type"])
}

func TestEnsureTypeFields_AdditionalPropertiesMissingType(t *testing.T) {
	t.Parallel()
	// Reproduces the Notion MCP notion-search tool schema where
	// filters.additionalProperties is an object schema without a "type" key.
	// OpenAI Responses API rejects schemas missing "type".
	schema := shared.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"filters": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					// No "type" key here — this is the bug
					"properties": map[string]any{
						"property": map[string]any{"type": "string"},
					},
				},
			},
		},
		"required": []any{"filters"},
	}

	updated := ensureTypeFields(schema)

	filters := updated["properties"].(map[string]any)["filters"].(map[string]any)
	additionalProps := filters["additionalProperties"].(map[string]any)
	assert.Equal(t, "object", additionalProps["type"], "additionalProperties should have type added")
}

func TestIsStrictCompatible(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		schema map[string]any
		want   bool
	}{
		{
			name:   "empty schema",
			schema: map[string]any{"type": "object"},
			want:   true,
		},
		{
			name: "explicit additionalProperties: false",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			},
			want: true,
		},
		{
			name: "additionalProperties: true is incompatible",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			want: false,
		},
		{
			name: "schema-form additionalProperties is incompatible",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"children": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
			},
			want: false,
		},
		{
			name: "schema-form additionalProperties nested in prefixItems is incompatible",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tuple": map[string]any{
						"type": "array",
						"prefixItems": []any{
							map[string]any{
								"type":                 "object",
								"additionalProperties": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isStrictCompatible(tc.schema))
		})
	}
}

func TestConvertParametersToSchema_NotionStylePreservesShape(t *testing.T) {
	t.Parallel()
	// Notion MCP tools declare schema-form additionalProperties so the model
	// knows what dictionary values look like. We must not strip that, even
	// though it forces non-strict mode. The inner schema is still normalized
	// (all properties become required) so the Chat Completions API receives a
	// fully-populated schema.
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"children": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"bulleted_list_item": map[string]any{"type": "string"},
						"numbered_list_item": map[string]any{"type": "string"},
					},
					"required": []any{"bulleted_list_item"},
				},
			},
		},
		"required": []any{"children"},
	}

	result, strict, err := ConvertParametersToSchema(schema)
	require.NoError(t, err)
	require.False(t, strict)

	children := result["properties"].(map[string]any)["children"].(map[string]any)
	inner, ok := children["additionalProperties"].(map[string]any)
	require.True(t, ok, "non-strict path must preserve schema-form additionalProperties")
	props := inner["properties"].(map[string]any)
	assert.Contains(t, props, "bulleted_list_item")
	assert.Contains(t, props, "numbered_list_item")

	// makeAllRequired still runs: every inner property is required so the
	// Chat Completions API gets a fully-populated schema.
	innerRequired := inner["required"].([]any)
	assert.Len(t, innerRequired, 2)
	assert.Contains(t, innerRequired, "bulleted_list_item")
	assert.Contains(t, innerRequired, "numbered_list_item")
}

func TestConvertParametersToSchema_AdditionalPropertiesMissingType(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filters": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"properties": map[string]any{
						"property": map[string]any{"type": "string"},
					},
				},
			},
		},
		"required": []any{"filters"},
	}

	result, strict, err := ConvertParametersToSchema(schema)
	require.NoError(t, err)

	// Schema-form additionalProperties => non-strict; the inner schema is
	// preserved (and ensureTypeFields adds the missing "type").
	assert.False(t, strict)
	filters := result["properties"].(map[string]any)["filters"].(map[string]any)
	additionalProps, ok := filters["additionalProperties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", additionalProps["type"])
}

func TestFixSchemaArrayItems(t *testing.T) {
	t.Parallel()
	schema := `{
  "properties": {
    "arguments": {
      "description": "Arguments to pass to the tool (can be any valid JSON value)",
      "type": [
        "string",
        "number",
        "boolean",
        "object",
        "array",
        "null"
      ]
    },
    "name": {
      "description": "Name of the tool to execute",
      "type": "string"
    }
  },
  "required": [
    "name"
  ],
  "type": "object"
}`

	schemaMap := map[string]any{}
	err := json.Unmarshal([]byte(schema), &schemaMap)
	require.NoError(t, err)

	updatedSchema := fixSchemaArrayItems(schemaMap)
	buf, err := json.Marshal(updatedSchema)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "properties": {
    "arguments": {
      "description": "Arguments to pass to the tool (can be any valid JSON value)",
      "type": [
        "string",
        "number",
        "boolean",
        "object",
        "array",
        "null"
      ]
    },
    "name": {
      "description": "Name of the tool to execute",
      "type": "string"
    }
  },
  "required": [
    "name"
  ],
  "type": "object"
}`, string(buf))
}
