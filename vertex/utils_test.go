package vertex

import (
	"reflect"
	"testing"

	"cloud.google.com/go/vertexai/genai"
	"oss.nandlabs.io/golly/data"
)

func TestToVertexSchema(t *testing.T) {
	tests := []struct {
		name     string
		inSchema *data.Schema
		want     *genai.Schema
	}{
		{
			name: "Bool Schema",
			inSchema: &data.Schema{
				Type: data.SchemaTypeBool,
			},
			want: &genai.Schema{
				Type: genai.TypeBoolean,
			},
		},
		{
			name: "String Schema with Enum and Pattern",
			inSchema: &data.Schema{
				Type:    data.SchemaTypeString,
				Enum:    []interface{}{"one", "two"},
				Pattern: stringPtr("^[a-z]+$"),
			},
			want: &genai.Schema{
				Type:    genai.TypeString,
				Enum:    []string{"one", "two"},
				Pattern: "^[a-z]+$",
			},
		},
		{
			name: "Number Schema with Format",
			inSchema: &data.Schema{
				Type:   data.SchemaTypeNumber,
				Format: stringPtr("float"),
			},
			want: &genai.Schema{
				Type:   genai.TypeNumber,
				Format: "float",
			},
		},
		{
			name: "Object Schema with Properties",
			inSchema: &data.Schema{
				Type: data.SchemaTypeObject,
				Properties: map[string]*data.Schema{
					"prop1": {
						Type: data.SchemaTypeString,
					},
				},
			},
			want: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"prop1": {
						Type: genai.TypeString,
					},
				},
			},
		},
		{
			name: "Array Schema with Items",
			inSchema: &data.Schema{
				Type: data.SchemaTypeArray,
				Items: &data.Schema{
					Type: data.SchemaTypeInteger,
				},
			},
			want: &genai.Schema{
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeInteger,
				},
			},
		},
		{
			name: "Integer Schema with Format",
			inSchema: &data.Schema{
				Type:   data.SchemaTypeInteger,
				Format: stringPtr("int32"),
			},
			want: &genai.Schema{
				Type:   genai.TypeInteger,
				Format: "int32",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToVertexSchema(tt.inSchema); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToVertexSchema() = %v, want %v", got, tt.want)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
