package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Violation 描述一次参数或配置校验失败，带 JSON Pointer。
type Violation struct {
	Pointer string
	Reason  string
}

func (v Violation) Error() string {
	if v.Pointer == "" {
		return v.Reason
	}
	return v.Pointer + ": " + v.Reason
}

// MaxArgumentBytes 限制模型传入的 arguments 原文大小。
const MaxArgumentBytes = 16 << 10

var schemaNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateSchemaDocument 在注册时校验 InputSchema / ConfigSchema 只使用受支持子集。
func ValidateSchemaDocument(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("schema 不是合法 JSON")
	}
	return validateSchemaNode("$", doc, true)
}

// ValidateJSON 按 schema 校验实例，返回全部违反项。
func ValidateJSON(schema, data json.RawMessage) []Violation {
	if len(schema) == 0 {
		return nil
	}
	var sch, val any
	if err := json.Unmarshal(schema, &sch); err != nil {
		return []Violation{{Pointer: "", Reason: "schema 不是合法 JSON"}}
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(data, &val); err != nil {
		return []Violation{{Pointer: "", Reason: "参数不是合法 JSON"}}
	}
	out := []Violation{}
	validateInstance("", sch, val, &out)
	return out
}

func validateSchemaNode(path string, node any, root bool) error {
	obj, ok := node.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema 必须是对象", path)
	}
	for key := range obj {
		if !schemaKeywordAllowed(key, root) {
			return fmt.Errorf("%s: 不支持的关键字 %q", path, key)
		}
	}
	typ, hasType := obj["type"]
	types := typeList(typ, hasType)
	if len(types) == 0 && !root {
		// 允许仅由 properties 推断 object
		if _, ok := obj["properties"]; ok {
			types = []string{"object"}
		}
	}
	for _, t := range types {
		if !allowedType(t) {
			return fmt.Errorf("%s: 不支持的 type %q", path, t)
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		for name, sub := range props {
			if !schemaNamePattern.MatchString(name) {
				return fmt.Errorf("%s/properties/%s: 属性名无效", path, name)
			}
			if err := validateSchemaNode(path+"/properties/"+name, sub, false); err != nil {
				return err
			}
		}
	}
	if add, ok := obj["additionalProperties"]; ok {
		switch add.(type) {
		case bool:
		default:
			return fmt.Errorf("%s: additionalProperties 必须是布尔值", path)
		}
	}
	if items, ok := obj["items"]; ok {
		if err := validateSchemaNode(path+"/items", items, false); err != nil {
			return err
		}
	}
	if enum, ok := obj["enum"].([]any); ok {
		for i, v := range enum {
			if v == nil {
				return fmt.Errorf("%s/enum/%d: enum 不能含 null", path, i)
			}
		}
	}
	if pat, ok := obj["pattern"].(string); ok {
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("%s: pattern 不是合法 RE2: %v", path, err)
		}
	}
	return nil
}

func schemaKeywordAllowed(key string, root bool) bool {
	switch key {
	case "type", "properties", "required", "additionalProperties", "enum", "const",
		"minLength", "maxLength", "pattern", "minimum", "maximum", "minItems", "maxItems",
		"items", "description", "default", "title":
		return true
	default:
		return false
	}
}

func allowedType(t string) bool {
	switch t {
	case "object", "string", "integer", "number", "boolean", "array", "null":
		return true
	default:
		return false
	}
}

func typeList(typ any, hasType bool) []string {
	if !hasType {
		return nil
	}
	switch v := typ.(type) {
	case string:
		return []string{v}
	case []any:
		out := []string{}
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func validateInstance(ptr string, schema any, value any, out *[]Violation) {
	obj, ok := schema.(map[string]any)
	if !ok {
		*out = append(*out, Violation{Pointer: ptr, Reason: "内部 schema 无效"})
		return
	}
	types := typeList(obj["type"], obj["type"] != nil)
	if len(types) == 0 {
		if _, ok := obj["properties"]; ok {
			types = []string{"object"}
		}
	}
	if len(types) > 0 && !valueMatchesTypes(value, types) {
		*out = append(*out, Violation{Pointer: ptr, Reason: "类型不匹配"})
		return
	}
	if c, ok := obj["const"]; ok && !jsonEqual(c, value) {
		*out = append(*out, Violation{Pointer: ptr, Reason: "必须等于常量"})
		return
	}
	if enum, ok := obj["enum"].([]any); ok {
		found := false
		for _, item := range enum {
			if jsonEqual(item, value) {
				found = true
				break
			}
		}
		if !found {
			*out = append(*out, Violation{Pointer: ptr, Reason: "不在 enum 允许范围内"})
		}
	}
	switch v := value.(type) {
	case string:
		if minL, ok := asInt(obj["minLength"]); ok && utf8.RuneCountInString(v) < minL {
			*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("长度不能少于 %d", minL)})
		}
		if maxL, ok := asInt(obj["maxLength"]); ok && utf8.RuneCountInString(v) > maxL {
			*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("长度超过 %d", maxL)})
		}
		if pat, ok := obj["pattern"].(string); ok {
			re := regexp.MustCompile(pat)
			if !re.MatchString(v) {
				*out = append(*out, Violation{Pointer: ptr, Reason: "不匹配 pattern"})
			}
		}
	case float64:
		checkNumber(ptr, obj, v, out)
	case json.Number:
		f, _ := v.Float64()
		checkNumber(ptr, obj, f, out)
	case map[string]any:
		props, _ := obj["properties"].(map[string]any)
		required := stringSet(obj["required"])
		additional := true
		if ap, ok := obj["additionalProperties"].(bool); ok {
			additional = ap
		}
		for key := range required {
			if _, ok := v[key]; !ok {
				*out = append(*out, Violation{Pointer: joinPtr(ptr, key), Reason: "必填字段缺失"})
			}
		}
		for key, val := range v {
			sub, ok := props[key]
			if !ok {
				if !additional {
					*out = append(*out, Violation{Pointer: joinPtr(ptr, key), Reason: "不允许额外字段"})
				}
				continue
			}
			validateInstance(joinPtr(ptr, key), sub, val, out)
		}
	case []any:
		if minI, ok := asInt(obj["minItems"]); ok && len(v) < minI {
			*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("元素数量不能少于 %d", minI)})
		}
		if maxI, ok := asInt(obj["maxItems"]); ok && len(v) > maxI {
			*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("元素数量不能超过 %d", maxI)})
		}
		if itemSch, ok := obj["items"]; ok {
			for i, item := range v {
				validateInstance(ptr+"/"+strconv.Itoa(i), itemSch, item, out)
			}
		}
	}
}

func checkNumber(ptr string, obj map[string]any, v float64, out *[]Violation) {
	if min, ok := asFloat(obj["minimum"]); ok && v < min {
		*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("必须 ≥ %v", min)})
	}
	if max, ok := asFloat(obj["maximum"]); ok && v > max {
		*out = append(*out, Violation{Pointer: ptr, Reason: fmt.Sprintf("必须 ≤ %v", max)})
	}
}

func valueMatchesTypes(value any, types []string) bool {
	for _, t := range types {
		if matchesType(value, t) {
			return true
		}
	}
	return false
}

func matchesType(value any, t string) bool {
	switch t {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, json.Number, int, int64:
			return true
		default:
			return false
		}
	case "integer":
		switch n := value.(type) {
		case float64:
			return n == float64(int64(n))
		case json.Number:
			_, err := n.Int64()
			return err == nil
		case int, int64:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func stringSet(v any) map[string]struct{} {
	out := map[string]struct{}{}
	list, ok := v.([]any)
	if !ok {
		return out
	}
	for _, item := range list {
		if s, ok := item.(string); ok {
			out[s] = struct{}{}
		}
	}
	return out
}

func joinPtr(base, key string) string {
	if base == "" {
		return "/" + escapePtr(key)
	}
	return base + "/" + escapePtr(key)
}

func escapePtr(s string) string {
	return strings.ReplaceAll(s, "~", "~0")
}
