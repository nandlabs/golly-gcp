package vertex

import (
	"cloud.google.com/go/vertexai/genai"
	"oss.nandlabs.io/golly/data"
)

// ToVertexSchema converts a data.Schema to a genai.Schema.
// This function is used to convert a data.Schema to a genai.Schema.
//
// Parameters:
//
//	*data.Schema: The input data.Schema to convert.
//
// Returns:
//
//	*genai.Schema: The converted genai.Schema.
func ToVertexSchema(inSchema *data.Schema) (outSchema *genai.Schema) {

	outSchema = &genai.Schema{}
	switch inSchema.Type {
	case data.SchemaTypeBool:
		outSchema.Type = genai.TypeBoolean
	case data.SchemaTypeString:
		outSchema.Type = genai.TypeString
		if inSchema.Format != nil {
			outSchema.Format = *inSchema.Format
		}
		if inSchema.Enum != nil {
			for _, val := range inSchema.Enum {

				v, ok := val.(string)
				if ok {
					outSchema.Enum = append(outSchema.Enum, v)
				}
			}
		}
		if inSchema.Pattern != nil {
			outSchema.Pattern = *inSchema.Pattern
		}

	case data.SchemaTypeNumber:
		outSchema.Type = genai.TypeNumber
		if inSchema.Format != nil {
			outSchema.Format = *inSchema.Format
		}
	case data.SchemaTypeObject:
		outSchema.Type = genai.TypeObject
		if inSchema.Properties != nil {
			outSchema.Properties = make(map[string]*genai.Schema)
			for key, value := range inSchema.Properties {
				outSchema.Properties[key] = ToVertexSchema(value)
			}
		}
		if inSchema.Required != nil {
			outSchema.Required = inSchema.Required
		}
		if inSchema.MinProperties != nil {
			outSchema.MinProperties = int64(*inSchema.MinProperties)
		}
		if inSchema.MaxProperties != nil {
			outSchema.MaxProperties = int64(*inSchema.MaxProperties)
		}

	case data.SchemaTypeArray:
		outSchema.Type = genai.TypeArray
		if inSchema.Items != nil {
			outSchema.Items = ToVertexSchema(inSchema.Items)
		}
	case data.SchemaTypeInteger:
		outSchema.Type = genai.TypeInteger
		if inSchema.Format != nil {
			outSchema.Format = *inSchema.Format
		}
	}

	outSchema.Description = inSchema.Description
	outSchema.Title = inSchema.Title
	outSchema.Nullable = inSchema.Nullable
	if inSchema.MinItems != nil {
		outSchema.MinItems = int64(*inSchema.MinItems)
	}
	if inSchema.MaxItems != nil {
		outSchema.MaxItems = int64(*inSchema.MaxItems)
	}
	if inSchema.Minimum != nil {
		outSchema.Minimum = *inSchema.Minimum
	}
	if inSchema.Maximum != nil {
		outSchema.Maximum = *inSchema.Maximum
	}
	if inSchema.MinLength != nil {
		outSchema.MinLength = int64(*inSchema.MinLength)
	}
	if inSchema.MaxLength != nil {
		outSchema.MaxLength = int64(*inSchema.MaxLength)
	}

	return
}
