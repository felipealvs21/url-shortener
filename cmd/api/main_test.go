package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string // substring expected in the Allow header, if any
	}{
		{name: "GET /healthz", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusOK},
		{name: "POST /healthz", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "GET unknown route", method: http.MethodGet, path: "/nao-existe", wantStatus: http.StatusNotFound},
	}

	router := newRouter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantAllow != "" {
				if got := rec.Header().Get("Allow"); !strings.Contains(got, tt.wantAllow) {
					t.Errorf("Allow = %q, want it to contain %q", got, tt.wantAllow)
				}
			}
		})
	}
}
