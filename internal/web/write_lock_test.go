package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteLockBusyRequestReturnsServiceUnavailable(t *testing.T) {
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: t.TempDir(),
		Token:     "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	writeMutex <- struct{}{}
	locked := true
	defer func() {
		if locked {
			<-writeMutex
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(response, req)
		close(done)
	}()

	select {
	case <-done:
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("request status = %d, want %d", response.Code, http.StatusServiceUnavailable)
		}
		if !strings.Contains(response.Body.String(), "中心正忙，请稍后重试") {
			t.Fatalf("busy response = %s, want Chinese retry message", response.Body)
		}
		if !strings.Contains(response.Body.String(), "write-busy") {
			t.Fatalf("busy response = %s, want write-busy code", response.Body)
		}
	case <-time.After(200 * time.Millisecond):
		<-writeMutex
		locked = false
		<-done
		t.Fatalf("request did not return within its deadline; after releasing the lock status = %d", response.Code)
	}
}

func TestUnconfirmedPullDoesNotWaitForWriteLock(t *testing.T) {
	server, err := NewServer(ServeOptions{
		Addr:      "127.0.0.1:0",
		HomerHome: t.TempDir(),
		Token:     "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	writeMutex <- struct{}{}
	locked := true
	defer func() {
		if locked {
			<-writeMutex
		}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/pull", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(response, req)
		close(done)
	}()

	select {
	case <-done:
		if response.Code != http.StatusConflict {
			t.Fatalf("unconfirmed pull status = %d, want %d; body=%s", response.Code, http.StatusConflict, response.Body)
		}
	case <-time.After(200 * time.Millisecond):
		<-writeMutex
		locked = false
		<-done
		t.Fatalf("unconfirmed pull waited for the write lock; after releasing the lock status = %d", response.Code)
	}
}
