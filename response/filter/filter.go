// Package filter provides query field filtering using dot-notation, index, and projection syntax.
package filter

import (
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// Node is one element of the selection tree built from a set of path strings.
// Each node corresponds to one path segment (e.g. "id" in "user.id").
type Node struct {
	Name     string
	Index    *int
	Children map[string]*Node
	IsLeaf   bool
}

// NewNode creates an empty selection tree node named name.
func NewNode(name string) *Node {
	return &Node{
		Name:     name,
		Children: make(map[string]*Node),
	}
}

// Filter returns a copy of data with fields selected or removed according to fields and omit.
// Supported path syntax:
//   - Dot notation: "data.items"
//   - Index notation: "items[0]"
//   - Projection: "items{id,name}"
//
// Usage:
//
//	filtered, err := filter.Filter(user, []string{"id", "name"}, nil)
func Filter(data any, fields []string, omit []string) (any, error) {
	if data == nil {
		return nil, nil
	}

	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr && val.IsNil() {
		return nil, nil
	}

	var includeRoot *Node
	inclusiveMode := false
	if len(fields) > 0 {
		includeRoot = NewNode("root")
		for _, f := range fields {
			parsePath(includeRoot, f)
		}
		inclusiveMode = true
	}

	var excludeRoot *Node
	if len(omit) > 0 {
		excludeRoot = NewNode("root")
		for _, o := range omit {
			parsePath(excludeRoot, o)
		}
	}

	if !inclusiveMode && excludeRoot == nil {
		return data, nil
	}

	return traverse(val, includeRoot, excludeRoot, inclusiveMode)
}

// traverse recursively filters data based on include/exclude selection trees.
func traverse(val reflect.Value, includeNode *Node, excludeNode *Node, inclusiveMode bool) (any, error) {
	for val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return nil, nil
		}
		val = val.Elem()
	}

	if excludeNode != nil && len(excludeNode.Children) == 0 && excludeNode.Index == nil {
		return nil, nil
	}

	effectiveInclusive := inclusiveMode
	if includeNode != nil && len(includeNode.Children) == 0 && includeNode.Index == nil {
		effectiveInclusive = false
		includeNode = nil
	}

	if effectiveInclusive && includeNode == nil {
		return nil, nil
	}

	if !effectiveInclusive && excludeNode == nil {
		return val.Interface(), nil
	}

	switch val.Kind() {
	case reflect.Struct:
		return traverseStruct(val, includeNode, excludeNode, effectiveInclusive)
	case reflect.Map:
		return traverseMap(val, includeNode, excludeNode, effectiveInclusive)
	case reflect.Slice, reflect.Array:
		return traverseSlice(val, includeNode, excludeNode, effectiveInclusive)
	default:
		return val.Interface(), nil
	}
}

// traverseStruct filters a struct's exported fields into a map[string]any,
// keyed by JSON tag name, honoring omitempty and the include/exclude trees.
func traverseStruct(val reflect.Value, includeNode *Node, excludeNode *Node, effectiveInclusive bool) (any, error) {
	result := make(map[string]any)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		if !fieldVal.CanInterface() {
			continue
		}

		fieldType := typ.Field(i)
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		tagParts := strings.Split(jsonTag, ",")
		name := tagParts[0]
		if name == "" {
			name = fieldType.Name
		}

		isOmitEmpty := false
		for _, part := range tagParts[1:] {
			if part == "omitempty" {
				isOmitEmpty = true
				break
			}
		}
		if isOmitEmpty && isEmptyValue(fieldVal) {
			continue
		}

		nextExclude, excluded := childExclude(excludeNode, name)
		if excluded {
			continue
		}

		nextInclude, included := childInclude(includeNode, name, effectiveInclusive)
		if !included {
			continue
		}

		res, err := traverse(fieldVal, nextInclude, nextExclude, effectiveInclusive)
		if err != nil {
			return nil, err
		}
		result[name] = res
	}
	return result, nil
}

// traverseMap filters a map's entries into a map[string]any, honoring the
// include/exclude trees. Keys are read via their string representation.
func traverseMap(val reflect.Value, includeNode *Node, excludeNode *Node, effectiveInclusive bool) (any, error) {
	result := make(map[string]any)

	for _, key := range val.MapKeys() {
		keyStr := key.String()

		nextExclude, excluded := childExclude(excludeNode, keyStr)
		if excluded {
			continue
		}

		nextInclude, included := childInclude(includeNode, keyStr, effectiveInclusive)
		if !included {
			continue
		}

		res, err := traverse(val.MapIndex(key), nextInclude, nextExclude, effectiveInclusive)
		if err != nil {
			return nil, err
		}
		result[keyStr] = res
	}
	return result, nil
}

// traverseSlice filters a slice/array's elements, supporting a single-index
// selection (e.g. "items[0]") or full iteration with per-index exclusion.
func traverseSlice(val reflect.Value, includeNode *Node, excludeNode *Node, effectiveInclusive bool) (any, error) {
	length := val.Len()

	if includeNode != nil && includeNode.Index != nil {
		idx := *includeNode.Index
		if idx < 0 || idx >= length {
			return nil, nil
		}
		nextInc := shallowCopyNodeWithoutIndex(includeNode)
		return traverse(val.Index(idx), nextInc, excludeNode, effectiveInclusive)
	}

	result := make([]any, 0, length)
	for i := 0; i < length; i++ {
		if excludeNode != nil && excludeNode.Index != nil && *excludeNode.Index == i {
			continue
		}

		res, err := traverse(val.Index(i), includeNode, excludeNode, effectiveInclusive)
		if err != nil {
			return nil, err
		}
		result = append(result, res)
	}
	return result, nil
}

// childExclude looks up name in excludeNode's children.
func childExclude(excludeNode *Node, name string) (*Node, bool) {
	if excludeNode == nil {
		return nil, false
	}
	child, ok := excludeNode.Children[name]
	if !ok {
		return nil, false
	}
	if len(child.Children) == 0 && child.Index == nil {
		return nil, true
	}
	return child, false
}

// childInclude looks up name in includeNode's children when effectiveInclusive is true.
func childInclude(includeNode *Node, name string, effectiveInclusive bool) (*Node, bool) {
	if !effectiveInclusive {
		return nil, true
	}
	child, ok := includeNode.Children[name]
	if !ok {
		return nil, false
	}
	return child, true
}

// isEmptyValue reports whether v holds its zero value, mirroring the rules
// encoding/json uses for the "omitempty" struct tag option.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

// shallowCopyNodeWithoutIndex copies n with Index cleared.
func shallowCopyNodeWithoutIndex(n *Node) *Node {
	return &Node{
		Name:     n.Name,
		Index:    nil,
		Children: n.Children,
		IsLeaf:   n.IsLeaf,
	}
}

// parsePath parses a dot-notation path string (e.g. "user.contacts[0].name")
// into nodes attached under root.
func parsePath(root *Node, path string) {
	current := root
	chars := []rune(path)
	i := 0
	for i < len(chars) {
		start := i
		for i < len(chars) && chars[i] != '.' && chars[i] != '[' && chars[i] != '{' && chars[i] != ',' && chars[i] != '}' {
			i++
		}
		segmentName := strings.TrimSpace(string(chars[start:i]))
		if segmentName != "" {
			if _, exists := current.Children[segmentName]; !exists {
				current.Children[segmentName] = NewNode(segmentName)
			}
			current = current.Children[segmentName]
		}
		if i >= len(chars) {
			break
		}

		switch chars[i] {
		case '.':
			i++
		case '[':
			i++
			startIdx := i
			for i < len(chars) && unicode.IsDigit(chars[i]) {
				i++
			}
			idxStr := string(chars[startIdx:i])
			if i < len(chars) && chars[i] == ']' {
				i++
			}
			if idx, err := strconv.Atoi(idxStr); err == nil {
				current.Index = &idx
			}
		case '{':
			i++
			balance := 1
			startBlock := i
			for i < len(chars) && balance > 0 {
				switch chars[i] {
				case '{':
					balance++
				case '}':
					balance--
				}
				i++
			}
			blockContent := string(chars[startBlock : i-1])
			for _, sp := range splitByCommaRespectingBraces(blockContent) {
				parsePath(current, sp)
			}
		case ',', '}':
			i++
		default:
			i++
		}
	}
}

// splitByCommaRespectingBraces splits s by commas outside projection blocks.
func splitByCommaRespectingBraces(s string) []string {
	var parts []string
	start := 0
	balance := 0
	for i, r := range s {
		switch r {
		case '{':
			balance++
		case '}':
			balance--
		case ',':
			if balance == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
