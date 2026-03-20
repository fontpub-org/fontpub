package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

var (
	protocolSchemaRootOnce sync.Once
	protocolSchemaRootPath string
)

func ValidateCLISchema(schemaFile string, value any) error {
	schema, err := loadCLISchema(schemaFile)
	if err != nil {
		return err
	}
	normalized, err := normalizeJSONValue(value)
	if err != nil {
		return err
	}
	return validateJSONSchema(schema, normalized, "$")
}

func loadCLISchema(schemaFile string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(protocolSchemaRoot(), "schemas", "cli", schemaFile))
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", schemaFile, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse schema %s: %w", schemaFile, err)
	}
	return schema, nil
}

func protocolSchemaRoot() string {
	protocolSchemaRootOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		protocolSchemaRootPath = filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "protocol"))
	})
	return protocolSchemaRootPath
}

func normalizeJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal value: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("unmarshal value: %w", err)
	}
	return normalized, nil
}

func validateJSONSchema(schema map[string]any, value any, path string) error {
	if len(schema) == 0 {
		return nil
	}

	if typeSpec, ok := schema["type"]; ok {
		if !matchesType(typeSpec, value) {
			return fmt.Errorf("%s: type mismatch", path)
		}
	}
	if constValue, ok := schema["const"]; ok {
		if !reflect.DeepEqual(value, constValue) {
			return fmt.Errorf("%s: const mismatch", path)
		}
	}
	if enumRaw, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range enumRaw {
			if reflect.DeepEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: enum mismatch", path)
		}
	}
	if allOfRaw, ok := schema["allOf"].([]any); ok {
		for i, raw := range allOfRaw {
			child, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: invalid allOf[%d]", path, i)
			}
			if err := validateJSONSchema(child, value, path); err != nil {
				return err
			}
		}
	}
	if ifRaw, ok := schema["if"].(map[string]any); ok {
		if matchesJSONSchema(ifRaw, value) {
			if thenRaw, ok := schema["then"].(map[string]any); ok {
				if err := validateJSONSchema(thenRaw, value, path); err != nil {
					return err
				}
			}
		}
	}
	if notRaw, ok := schema["not"].(map[string]any); ok {
		if matchesJSONSchema(notRaw, value) {
			return fmt.Errorf("%s: matched forbidden schema", path)
		}
	}

	obj, isObject := value.(map[string]any)
	if isObject {
		if requiredRaw, ok := schema["required"].([]any); ok {
			for _, raw := range requiredRaw {
				key, ok := raw.(string)
				if !ok {
					return fmt.Errorf("%s: invalid required key", path)
				}
				if _, exists := obj[key]; !exists {
					return fmt.Errorf("%s.%s: missing required property", path, key)
				}
			}
		}
		props := map[string]map[string]any{}
		if propsRaw, ok := schema["properties"].(map[string]any); ok {
			for key, raw := range propsRaw {
				if child, ok := raw.(map[string]any); ok {
					props[key] = child
					if childValue, exists := obj[key]; exists {
						if err := validateJSONSchema(child, childValue, path+"."+key); err != nil {
							return err
						}
					}
				}
			}
		}
		if additional, exists := schema["additionalProperties"]; exists {
			for key, childValue := range obj {
				if _, declared := props[key]; declared {
					continue
				}
				switch typed := additional.(type) {
				case bool:
					if !typed {
						return fmt.Errorf("%s.%s: additional property is not allowed", path, key)
					}
				case map[string]any:
					if err := validateJSONSchema(typed, childValue, path+"."+key); err != nil {
						return err
					}
				}
			}
		}
	}

	if itemsRaw, ok := schema["items"].(map[string]any); ok {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array items", path)
		}
		for i, item := range items {
			if err := validateJSONSchema(itemsRaw, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesJSONSchema(schema map[string]any, value any) bool {
	return validateJSONSchema(schema, value, "$") == nil
}

func matchesType(typeSpec any, value any) bool {
	switch typed := typeSpec.(type) {
	case string:
		return matchesNamedType(typed, value)
	case []any:
		for _, raw := range typed {
			name, ok := raw.(string)
			if ok && matchesNamedType(name, value) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func matchesNamedType(name string, value any) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32, uint, uint64, uint32:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func schemaFileNameForCLICommand(command string) string {
	switch strings.TrimSpace(command) {
	case "ls-remote":
		return "ls-remote-result.schema.json"
	case "ls":
		return "ls-result.schema.json"
	case "verify":
		return "verify-result.schema.json"
	case "repair":
		return "repair-result.schema.json"
	case "package init":
		return "package-init-result.schema.json"
	case "package preview":
		return "package-preview-result.schema.json"
	default:
		return ""
	}
}
