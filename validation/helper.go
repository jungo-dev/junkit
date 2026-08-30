package validation

import (
	"regexp"
	"strings"
)

var (
	matchFirstCap = regexp.MustCompile("([A-Z]+)([A-Z][a-z])")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

// camelToSnake converts a CamelCase or PascalCase string to snake_case.
//
// Usage:
//
//	camelToSnake("UserCreateRequest") // "user_create_request"
func camelToSnake(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// normalizeFieldPath converts a validator namespace into a snake_case JSON field path.
//
// Usage:
//
//	normalizeFieldPath("CreateUserRequest.Password") // "password"
func normalizeFieldPath(namespace string) string {
	root := strings.Split(namespace, ".")[0]
	rawPath := strings.TrimPrefix(namespace, root+".")

	parts := strings.Split(rawPath, ".")
	filteredParts := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "Request" {
			continue
		}

		if idx := strings.Index(part, "["); idx != -1 {
			base := camelToSnake(part[:idx])
			index := part[idx:]
			filteredParts = append(filteredParts, base+index)
		} else {
			filteredParts = append(filteredParts, camelToSnake(part))
		}
	}

	return strings.Join(filteredParts, ".")
}
