package docs

import (
	"fmt"
	"sort"
	"strings"
)

// Endpoint describes a single API endpoint for OpenAPI generation.
type Endpoint struct {
	Path        string
	Method      string // GET, POST, etc.
	Tag         string
	Summary     string
	OperationID string
	Description string

	// Parameters for query/path params (GET endpoints).
	Parameters []Parameter

	// RequestBody schema (POST endpoints).
	RequestBody *RequestBody

	// Responses keyed by status code.
	Responses map[string]Response
}

// Parameter describes a path or query parameter.
type Parameter struct {
	Name        string
	In          string // "query" or "path"
	Required    bool
	Description string
	Example     string
	Schema      Schema
}

// RequestBody describes a JSON request body.
type RequestBody struct {
	Required bool
	Schema   Schema
}

// Response describes an HTTP response.
type Response struct {
	Description string
	ContentType string // e.g. "application/json", "text/plain"
	Schema      *Schema
}

// Schema is a simplified OpenAPI schema representation.
type Schema struct {
	Type       string            // "object", "string", "integer", "boolean", "array", "number"
	Ref        string            // $ref value, e.g. "#/components/schemas/Error"
	Properties map[string]Schema // for object types
	Required   []string          // required property names
	Items      *Schema           // for array types
	Enum       []string          // enum values
	Nullable   bool
	Format     string
	Default    any
	Example    any
	Desc       string // description for inline schemas
}

// ComponentSchema is a named schema in components/schemas.
type ComponentSchema struct {
	Name   string
	Schema Schema
}

// Spec holds everything needed to generate a complete OpenAPI document.
type Spec struct {
	Title       string
	Version     string
	Description string
	ServerURL   string
	ServerDesc  string
	Tags        []Tag
	Endpoints   []Endpoint
	Components  []ComponentSchema
}

// Tag is an OpenAPI tag with description.
type Tag struct {
	Name        string
	Description string
}

// GenerateOpenAPI produces the full OpenAPI YAML from the spec.
func GenerateOpenAPI(spec Spec) []byte {
	var b strings.Builder

	b.WriteString("openapi: 3.1.0\n")
	b.WriteString("info:\n")
	b.WriteString(fmt.Sprintf("  title: %s\n", spec.Title))
	b.WriteString(fmt.Sprintf("  version: %s\n", spec.Version))
	if spec.Description != "" {
		b.WriteString("  description: |\n")
		for _, line := range strings.Split(spec.Description, "\n") {
			if line == "" {
				b.WriteString("\n")
			} else {
				b.WriteString("    " + line + "\n")
			}
		}
	}
	b.WriteString("\n")

	if spec.ServerURL != "" {
		b.WriteString("servers:\n")
		b.WriteString(fmt.Sprintf("  - url: %s\n", spec.ServerURL))
		if spec.ServerDesc != "" {
			b.WriteString(fmt.Sprintf("    description: %s\n", spec.ServerDesc))
		}
		b.WriteString("\n")
	}

	if len(spec.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range spec.Tags {
			b.WriteString(fmt.Sprintf("  - name: %s\n", tag.Name))
			if tag.Description != "" {
				b.WriteString(fmt.Sprintf("    description: %s\n", tag.Description))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("paths:\n")
	for _, ep := range spec.Endpoints {
		writeEndpoint(&b, ep)
	}

	if len(spec.Components) > 0 {
		b.WriteString("\ncomponents:\n")
		b.WriteString("  schemas:\n")
		for _, cs := range spec.Components {
			writeComponentSchema(&b, cs.Name, cs.Schema, 4)
		}
	}

	return []byte(b.String())
}

func writeEndpoint(b *strings.Builder, ep Endpoint) {
	b.WriteString(fmt.Sprintf("  %s:\n", ep.Path))
	method := strings.ToLower(ep.Method)
	b.WriteString(fmt.Sprintf("    %s:\n", method))
	b.WriteString(fmt.Sprintf("      tags: [%s]\n", ep.Tag))
	b.WriteString(fmt.Sprintf("      summary: %s\n", ep.Summary))
	b.WriteString(fmt.Sprintf("      operationId: %s\n", ep.OperationID))

	if ep.Description != "" {
		b.WriteString("      description: |\n")
		for _, line := range strings.Split(ep.Description, "\n") {
			if line == "" {
				b.WriteString("\n")
			} else {
				b.WriteString("        " + line + "\n")
			}
		}
	}

	if len(ep.Parameters) > 0 {
		b.WriteString("      parameters:\n")
		for _, p := range ep.Parameters {
			b.WriteString(fmt.Sprintf("        - name: %s\n", p.Name))
			b.WriteString(fmt.Sprintf("          in: %s\n", p.In))
			if p.Required {
				b.WriteString("          required: true\n")
			}
			writeSchemaInline(b, p.Schema, 10)
			if p.Description != "" {
				b.WriteString(fmt.Sprintf("          description: %s\n", p.Description))
			}
			if p.Example != "" {
				b.WriteString(fmt.Sprintf("          example: %q\n", p.Example))
			}
		}
	}

	if ep.RequestBody != nil {
		b.WriteString("      requestBody:\n")
		if ep.RequestBody.Required {
			b.WriteString("        required: true\n")
		}
		b.WriteString("        content:\n")
		b.WriteString("          application/json:\n")
		b.WriteString("            schema:\n")
		writeSchemaBlock(b, ep.RequestBody.Schema, 14)
	}

	if len(ep.Responses) > 0 {
		b.WriteString("      responses:\n")
		// Sort response codes for deterministic output.
		codes := make([]string, 0, len(ep.Responses))
		for code := range ep.Responses {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			resp := ep.Responses[code]
			b.WriteString(fmt.Sprintf("        %q:\n", code))
			b.WriteString(fmt.Sprintf("          description: %s\n", resp.Description))
			if resp.Schema != nil {
				ct := resp.ContentType
				if ct == "" {
					ct = "application/json"
				}
				b.WriteString("          content:\n")
				b.WriteString(fmt.Sprintf("            %s:\n", ct))
				b.WriteString("              schema:\n")
				writeSchemaBlock(b, *resp.Schema, 16)
			}
		}
	}
}

func writeSchemaInline(b *strings.Builder, s Schema, indent int) {
	prefix := strings.Repeat(" ", indent)
	if s.Ref != "" {
		b.WriteString(fmt.Sprintf("%sschema:\n", prefix))
		b.WriteString(fmt.Sprintf("%s  $ref: %q\n", prefix, s.Ref))
		return
	}
	b.WriteString(fmt.Sprintf("%sschema:\n", prefix))
	b.WriteString(fmt.Sprintf("%s  type: %s\n", prefix, s.Type))
}

func writeSchemaBlock(b *strings.Builder, s Schema, indent int) {
	prefix := strings.Repeat(" ", indent)
	if s.Ref != "" {
		b.WriteString(fmt.Sprintf("%s$ref: %q\n", prefix, s.Ref))
		return
	}

	b.WriteString(fmt.Sprintf("%stype: %s\n", prefix, s.Type))
	if s.Nullable {
		b.WriteString(fmt.Sprintf("%snullable: true\n", prefix))
	}
	if s.Format != "" {
		b.WriteString(fmt.Sprintf("%sformat: %s\n", prefix, s.Format))
	}
	if s.Desc != "" {
		b.WriteString(fmt.Sprintf("%sdescription: %s\n", prefix, s.Desc))
	}
	if s.Default != nil {
		b.WriteString(fmt.Sprintf("%sdefault: %v\n", prefix, s.Default))
	}
	if s.Example != nil {
		switch v := s.Example.(type) {
		case string:
			b.WriteString(fmt.Sprintf("%sexample: %q\n", prefix, v))
		default:
			b.WriteString(fmt.Sprintf("%sexample: %v\n", prefix, v))
		}
	}

	if len(s.Enum) > 0 {
		b.WriteString(fmt.Sprintf("%senum: [%s]\n", prefix, strings.Join(s.Enum, ", ")))
	}

	if len(s.Required) > 0 {
		b.WriteString(fmt.Sprintf("%srequired: [%s]\n", prefix, strings.Join(s.Required, ", ")))
	}

	if len(s.Properties) > 0 {
		b.WriteString(fmt.Sprintf("%sproperties:\n", prefix))
		// Sort properties for deterministic output.
		propNames := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)
		for _, name := range propNames {
			prop := s.Properties[name]
			b.WriteString(fmt.Sprintf("%s  %s:\n", prefix, name))
			writeSchemaBlock(b, prop, indent+4)
		}
	}

	if s.Items != nil {
		b.WriteString(fmt.Sprintf("%sitems:\n", prefix))
		writeSchemaBlock(b, *s.Items, indent+2)
	}
}

func writeComponentSchema(b *strings.Builder, name string, s Schema, indent int) {
	prefix := strings.Repeat(" ", indent)
	b.WriteString(fmt.Sprintf("%s%s:\n", prefix, name))
	writeSchemaBlock(b, s, indent+2)
}
