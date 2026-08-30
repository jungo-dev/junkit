// Package validation wraps go-playground/validator with custom rules and localized error messages.
package validation

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"github.com/jungo-dev/junkit/i18n"
)

// Options configures the Validator instance.
type Options struct {
	// Language is the default locale used to resolve validation message keys, e.g. "en".
	Language string
}

// Validator translates binding and validation errors into localized, field-keyed messages.
type Validator struct {
	translator *i18n.Translator
	lang       string
}

// NewValidator creates a new Validator instance with custom rules registered.
//
// Usage:
//
//	v := validation.NewValidator(validation.Options{Language: "en"}, translator)
func NewValidator(opts Options, translator *i18n.Translator) *Validator {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		RegisterCustomValidation(v)
	}

	return &Validator{
		translator: translator,
		lang:       opts.Language,
	}
}

// GetValidationErrors converts binding errors into localized field error maps or request error strings.
//
// Usage:
//
//	if err := ctx.ShouldBindJSON(&req); err != nil {
//	    errs := v.GetValidationErrors(ctx, err)
//	    responder.SendWithData(ctx, http.StatusUnprocessableEntity, "validation_error", errs)
//	    return
//	}
func (v *Validator) GetValidationErrors(ctx *gin.Context, err error) any {
	if err == nil {
		return nil
	}

	if errMap, ok := v.jsonTypeError(err); ok {
		return errMap
	}

	if result, ok := v.numError(ctx, err); ok {
		return result
	}

	if errMap, ok := v.fieldValidationErrors(err); ok {
		return errMap
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return v.translate("invalid_json_syntax", "invalid json syntax")
	}

	return v.translate("invalid_request", "invalid request")
}

// jsonTypeError handles json.UnmarshalTypeError when a field receives an invalid data type.
func (v *Validator) jsonTypeError(err error) (map[string]string, bool) {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return nil, false
	}

	field := camelToSnake(typeErr.Field)
	typeName := typeErr.Type.String()
	friendlyType := v.translate("type_"+typeName, typeName)

	return map[string]string{
		field: v.translate("invalid_type", "invalid type", field, friendlyType),
	}, true
}

// numError handles strconv.NumError produced during query or form primitive type conversion.
func (v *Validator) numError(ctx *gin.Context, err error) (any, bool) {
	var numErr *strconv.NumError
	if !errors.As(err, &numErr) {
		return nil, false
	}

	val := numErr.Num
	typeName := "int"
	switch {
	case strings.Contains(numErr.Func, "Bool"):
		typeName = "bool"
	case strings.Contains(numErr.Func, "Float"):
		typeName = "float64"
	}
	friendlyType := v.translate("type_"+typeName, typeName)

	if ctx != nil {
		if fieldName := findQueryOrFormKey(ctx, val); fieldName != "" {
			return map[string]string{
				fieldName: v.translate("invalid_type", "invalid type", fieldName, friendlyType),
			}, true
		}
	}

	return v.translate("invalid_type_value", "Invalid value '%s' for type %s", val, friendlyType), true
}

// findQueryOrFormKey searches request query parameters and form values for val.
func findQueryOrFormKey(ctx *gin.Context, val string) string {
	for key, values := range ctx.Request.URL.Query() {
		if slices.Contains(values, val) {
			return key
		}
	}
	for key, values := range ctx.Request.PostForm {
		if slices.Contains(values, val) {
			return key
		}
	}
	return ""
}

// fieldValidationErrors handles validator.ValidationErrors for failed struct-tag rules.
func (v *Validator) fieldValidationErrors(err error) (map[string]string, bool) {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return nil, false
	}

	errMap := make(map[string]string)
	for _, e := range validationErrs {
		fieldPath := normalizeFieldPath(e.Namespace())
		if _, exists := errMap[fieldPath]; exists {
			continue
		}

		tag := e.Tag()
		if (tag == "min" || tag == "max") && isNumericKind(e.Type().Kind()) {
			tag += "_int"
		}
		errMap[fieldPath] = v.translate(tag, tag, fieldPath, e.Param())
	}

	return errMap, true
}

// isNumericKind reports whether reflect.Kind is a number.
func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// translate resolves key via translator, returning fallback if missing.
func (v *Validator) translate(key, fallback string, args ...any) string {
	if v.translator == nil {
		return fallback
	}
	msg := v.translator.GetMessage(key, v.lang, args...)
	if msg == key && fallback != "" {
		return fallback
	}
	return msg
}
