package middleware_test

import (
	"sync"

	"github.com/gin-gonic/gin"
)

// errorNotification is one call recorded by fakeNotifier.SendErrorNotification.
type errorNotification struct {
	data       map[string]any
	statusCode int
}

// fakeNotifier mocks notification.Notifier by recording sent messages for testing.
type fakeNotifier struct {
	messages chan string

	mu    sync.Mutex
	sends []errorNotification
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{messages: make(chan string, 8)}
}

func (f *fakeNotifier) SendMessageHTML(_ *gin.Context, message string, _ string) {
	f.messages <- message
}

func (f *fakeNotifier) SendErrorNotification(_ *gin.Context, errData map[string]any, statusCode int, _ ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, errorNotification{data: errData, statusCode: statusCode})
}

func (f *fakeNotifier) errorNotifications() []errorNotification {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]errorNotification, len(f.sends))
	copy(out, f.sends)
	return out
}
