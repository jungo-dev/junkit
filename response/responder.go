package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/i18n"
	"github.com/jungo-dev/junkit/pagination"
	"github.com/jungo-dev/junkit/response/filter"
)

// Options configures a Responder.
type Options struct {
	// Language is the default locale for i18n messages (e.g. "en").
	Language string
}

// Responder formats and sends localized JSON responses.
type Responder interface {
	// Send writes a message-only response.
	Send(ctx *gin.Context, statusCode int, key string, args ...any)
	// SendWithData writes a response carrying data.
	SendWithData(ctx *gin.Context, statusCode int, key string, data any, args ...any)
	// Pagination writes a paginated list response with metadata.
	Pagination(ctx *gin.Context, statusCode int, key string, data any, pagination pagination.Pagination, args ...any)
	// Translate resolves an i18n key to a localized string.
	Translate(key string, args ...any) string
	// Error maps err to an HTTP status and JSON response.
	Error(ctx *gin.Context, err error)
}

// responder is the default Responder implementation.
type responder struct {
	translator *i18n.Translator
	lang       string
}

// NewResponder creates a Responder with the given Options and i18n Translator.
//
// Usage:
//
//	responder := response.NewResponder(response.Options{Language: "en"}, translator)
func NewResponder(opts Options, translator *i18n.Translator) Responder {
	return &responder{
		translator: translator,
		lang:       opts.Language,
	}
}

// Send writes a message-only response.
//
// Usage:
//
//	responder.Send(ctx, http.StatusOK, "operation_successful")
func (r *responder) Send(ctx *gin.Context, statusCode int, key string, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	ctx.JSON(statusCode, Response{
		Status:  statusText(statusCode),
		Message: message,
		APIInfo: getAPIInfo(ctx),
	})
}

// SendWithData writes a response with data in "data" (status < 400) or "error" (status >= 400).
//
// Usage:
//
//	responder.SendWithData(ctx, http.StatusOK, "user_fetched", user)
func (r *responder) SendWithData(ctx *gin.Context, statusCode int, key string, data any, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	res := Response{
		Status:  statusText(statusCode),
		Message: message,
		APIInfo: getAPIInfo(ctx),
	}

	if statusCode >= http.StatusBadRequest {
		res.Error = data
	} else {
		res.Data = data
	}

	r.renderJSON(ctx, statusCode, res)
}

// Pagination writes a paginated list response with items and paging metadata.
//
// Usage:
//
//	responder.Pagination(ctx, http.StatusOK, "users_listed", users, meta)
func (r *responder) Pagination(ctx *gin.Context, statusCode int, key string, data any, pagination pagination.Pagination, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	res := Response{
		Status:  statusText(statusCode),
		Message: message,
		Data: PaginatedData{
			Items:      data,
			Pagination: pagination,
		},
		APIInfo: getAPIInfo(ctx),
	}

	r.renderJSON(ctx, statusCode, res)
}

// Translate resolves an i18n key to a localized string.
//
// Usage:
//
//	msg := responder.Translate("welcome_message", userName)
func (r *responder) Translate(key string, args ...any) string {
	return r.translator.GetMessage(key, r.lang, args...)
}

// Error maps err to an HTTP response (*AppError maps its Code/Message, generic errors map to 500).
//
// Usage:
//
//	if err != nil {
//	    responder.Error(ctx, err)
//	    return
//	}
func (r *responder) Error(ctx *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		statusCode := errorCodeToHTTPStatus(appErr.Code)
		message := r.translator.GetMessage(appErr.Message, r.lang)

		ctx.JSON(statusCode, Response{
			Status:  "error",
			Code:    appErr.Code,
			Message: message,
			APIInfo: getAPIInfo(ctx),
		})
		return
	}

	_ = ctx.Error(err)
	r.Send(ctx, http.StatusInternalServerError, "internal_server_error")
}

// renderJSON applies "?fields="/"?omit=" filtering to successful responses and writes JSON.
func (r *responder) renderJSON(ctx *gin.Context, statusCode int, res Response) {
	if statusCode < http.StatusBadRequest {
		fields := ctx.GetStringSlice("_response_fields")
		omit := ctx.GetStringSlice("_response_omit")

		if len(fields) > 0 || len(omit) > 0 {
			if filtered, err := filter.Filter(res, fields, omit); err == nil {
				ctx.JSON(statusCode, filtered)
				return
			}
		}
	}

	ctx.JSON(statusCode, res)
}

// errorCodeToHTTPStatus maps ErrorCode to HTTP status, defaulting to 500.
func errorCodeToHTTPStatus(code ErrorCode) int {
	switch code {
	case BadRequest:
		return http.StatusBadRequest
	case Unauthorized:
		return http.StatusUnauthorized
	case NotFound:
		return http.StatusNotFound
	case Forbidden:
		return http.StatusForbidden
	case Conflict:
		return http.StatusConflict
	case UnsupportedMediaType:
		return http.StatusUnsupportedMediaType
	case TooManyRequests:
		return http.StatusTooManyRequests
	case UnprocessableEntity:
		return http.StatusUnprocessableEntity
	case Internal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// statusText returns "error" for status >= 400, "success" otherwise.
func statusText(code int) string {
	if code >= http.StatusBadRequest {
		return "error"
	}
	return "success"
}
