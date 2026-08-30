// Package notification sends operational alerts to Telegram.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/telegram"
)

// Notifier sends operational alerts to external notification channels.
type Notifier interface {
	SendMessageHTML(c *gin.Context, message string, typeMessage string)
	SendErrorNotification(c *gin.Context, errData map[string]any, statusCode int, sql ...string)
}

// Options configures the notification service.
type Options struct {
	// TelegramChatID is the chat every alert is sent to.
	TelegramChatID string
	// Environment gates SendErrorNotification: alerts are only sent when this equals "production".
	Environment string
}

// notifier is the default implementation of Notifier.
type notifier struct {
	tgClient telegram.Client
	opts     Options
}

// NewNotifier creates a Notifier instance.
//
// Usage:
//
//	n := notification.NewNotifier(tgClient, notification.Options{
//	    TelegramChatID: cfg.TelegramChatID,
//	    Environment:    cfg.Environment,
//	})
func NewNotifier(tgClient telegram.Client, opts Options) Notifier {
	return &notifier{tgClient: tgClient, opts: opts}
}

// SendMessageHTML implements Notifier.
func (n *notifier) SendMessageHTML(_ *gin.Context, message string, typeMessage string) {
	var sb strings.Builder
	if typeMessage == "error" {
		sb.WriteString("🚨 <b>Error</b> 🚨\n\n")
	} else {
		sb.WriteString("🟠 <b>Warning</b> 🟠\n\n")
	}
	sb.WriteString(message)

	_ = n.tgClient.SendMessage(context.Background(), n.opts.TelegramChatID, sb.String())
}

// SendErrorNotification sends an error report to Telegram (production environment only).
func (n *notifier) SendErrorNotification(c *gin.Context, errData map[string]any, statusCode int, sql ...string) {
	if n.opts.Environment != "production" {
		return
	}

	ctx := context.Background()
	notificationText := buildErrorNotification(c, errData, statusCode, sql)

	if stack, ok := errData["stack"].(string); ok && stack != "" {
		n.sendWithStackTrace(ctx, notificationText, stack)
		return
	}

	_ = n.tgClient.SendMessage(ctx, n.opts.TelegramChatID, notificationText.String())
}

// buildErrorNotification renders the HTML error message body.
func buildErrorNotification(c *gin.Context, errData map[string]any, statusCode int, sql []string) *strings.Builder {
	var sb strings.Builder

	if statusCode >= 500 {
		sb.WriteString("🚨 <b>Error Notification</b> 🚨\n\n")
	} else {
		sb.WriteString("🟠 <b>Client Error</b> 🟠\n\n")
	}

	sb.WriteString(fmt.Sprintf(
		"🛜 <b>User Agent:</b> %s\n"+
			"💻 <b>IP:</b> %s\n"+
			"📡 <b>Status:</b> %d\n"+
			"📦 <b>Method:</b> %s\n\n",
		escapeTelegram(userAgent(c)),
		c.ClientIP(),
		statusCode,
		c.Request.Method,
	))

	sb.WriteString(fmt.Sprintf(
		"📝 <b>Location:</b>\n<pre>"+
			"File: %s\n"+
			"Function: %s\n"+
			"Line: %v\n"+
			"Trace ID: %s"+
			"</pre>\n\n",
		escapeTelegram(getString(errData, "location")),
		escapeTelegram(getString(errData, "function")),
		errData["line"],
		getTraceID(c),
	))

	sb.WriteString(fmt.Sprintf(
		"🐞 <b>Error:</b>\n<pre>%s</pre>\n\n",
		escapeTelegram(getString(errData, "error")),
	))

	if curl := generateCurlCommand(c); curl != "" {
		sb.WriteString(fmt.Sprintf("🍺 <b>cURL Command:</b>\n<pre>%s</pre>\n", escapeTelegram(curl)))
	}

	if len(sql) > 0 && sql[0] != "" {
		sb.WriteString(fmt.Sprintf("\n💾 <b>SQL:</b>\n<pre>%s</pre>", escapeTelegram(sql[0])))
	}

	return &sb
}

// sendWithStackTrace sends an alert message with stack trace attached as a document.
func (n *notifier) sendWithStackTrace(ctx context.Context, caption *strings.Builder, stack string) {
	stackContent := fmt.Sprintf(
		"STACK TRACE\nGenerated: %s\n----------------------------------------\n\n%s",
		time.Now().Format("2006-01-02 15:04:05 MST"),
		stack,
	)

	buf := bytes.NewBufferString(stackContent)
	filename := fmt.Sprintf("stack-%s.txt", time.Now().Format("20060102-150405"))
	fullCaption := caption.String() + "\n\n📄 <b>Full stack trace attached above</b>"

	_ = n.tgClient.SendDocument(ctx, n.opts.TelegramChatID, buf, filename, fullCaption)
}

// getTraceID reads the request trace ID set by the trace middleware.
func getTraceID(c *gin.Context) string {
	if v, exists := c.Get("trace_id"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "-"
}

// userAgent returns the request's User-Agent header, or "-" if absent.
func userAgent(c *gin.Context) string {
	if ua := c.Request.UserAgent(); ua != "" {
		return ua
	}
	return "-"
}

// getString returns m[key] as a string, or "-" if absent or not a string.
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "-"
}

// escapeTelegram escapes special HTML characters for Telegram parse mode.
func escapeTelegram(s string) string {
	if s == "" {
		return ""
	}
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// generateCurlCommand renders a reproducible cURL command for a request.
func generateCurlCommand(c *gin.Context) string {
	fullURL := getFullRequestURL(c)
	parts := []string{fmt.Sprintf("curl -X %s %q", c.Request.Method, fullURL)}

	skipHeaders := map[string]bool{
		"content-length": true,
		"user-agent":     true,
		"postman-token":  true,
		"cookie":         true,
	}

	for k, values := range c.Request.Header {
		if skipHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range values {
			parts = append(parts, fmt.Sprintf("-H %q", k+": "+v))
		}
	}

	if body := readAndRestoreBody(c); body != "" {
		parts = append(parts, fmt.Sprintf("-d '%s'", body))
	}

	if len(parts) <= 1 {
		return parts[0]
	}

	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteString(" \\\n  ")
		}
		sb.WriteString(p)
	}
	return sb.String()
}

// readAndRestoreBody reads and restores the request body for cURL generation.
func readAndRestoreBody(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if err != nil || len(bodyBytes) == 0 {
		return ""
	}

	pretty := string(bodyBytes)
	if json.Unmarshal(bodyBytes, new(any)) == nil {
		if prettyJSON, err := json.MarshalIndent(json.RawMessage(bodyBytes), "", "  "); err == nil {
			pretty = string(prettyJSON)
		}
	}

	return strings.ReplaceAll(pretty, "'", "'\\''")
}

// getFullRequestURL reconstructs the request's absolute URL, honoring X-Forwarded-Proto.
func getFullRequestURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return fmt.Sprintf("%s://%s%s", scheme, c.Request.Host, c.Request.RequestURI)
}
