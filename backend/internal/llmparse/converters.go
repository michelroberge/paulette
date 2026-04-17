package llmparse

import (
	"encoding/json"
	"reflect"
	"strings"
)

// -----------------------------
// List → Struct converter
// -----------------------------
type StringListToStruct struct{}

func (c StringListToStruct) Name() string {
	return "string_list_to_struct"
}

func (c StringListToStruct) Supports(input any, target any) float64 {
	_, ok := input.([]string)
	if !ok {
		return 0
	}
	rt := reflect.TypeOf(target)
	if rt.Kind() != reflect.Ptr || rt.Elem().Kind() != reflect.Struct {
		return 0
	}
	return 0.8
}

func (c StringListToStruct) Convert(input any, target any) error {
	list := input.([]string)
	return assignStringSliceToStructField(list, target)
}

// -----------------------------
// KV → Struct converter
// -----------------------------
type KVToStruct struct{}

func (c KVToStruct) Name() string { return "kv_to_struct" }

func (c KVToStruct) Supports(input any, target any) float64 {
	s, ok := input.(string)
	if !ok {
		return 0
	}
	if strings.Contains(s, ":") {
		return 0.7
	}
	return 0
}

func (c KVToStruct) Convert(input any, target any) error {
	s := input.(string)
	lines := strings.Split(s, "\n")
	m := map[string]string{}
	for _, line := range lines {
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			m[key] = val
		}
	}
	data, _ := json.Marshal(m)
	return json.Unmarshal(data, target)
}
