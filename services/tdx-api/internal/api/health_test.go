package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"KlineChartQuantGo/services/tdx-api/internal/client"
)

func TestLivenessAlwaysReturnsOK(t *testing.T) {
	router := newRouterWithStatus(nil, func() client.Status { return client.Status{} })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestReadinessReturnsServiceUnavailableWhenAnyDomainIsDown(t *testing.T) {
	status := client.Status{Ready: false, Domains: map[client.Domain]client.DomainStatus{
		client.DomainMain: {Ready: true},
		client.DomainEx:   {Ready: false, LastError: "unavailable"},
	}}
	router := newRouterWithStatus(nil, func() client.Status { return status })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

func TestReadinessReturnsOKWhenAllDomainsAreReady(t *testing.T) {
	status := client.Status{Ready: true, Domains: map[client.Domain]client.DomainStatus{
		client.DomainMain: {Ready: true},
		client.DomainEx:   {Ready: true},
		client.DomainMAC:  {Ready: true},
	}}
	router := newRouterWithStatus(nil, func() client.Status { return status })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}
