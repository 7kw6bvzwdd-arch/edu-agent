package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelGatewayHTTP1Compatibility(t *testing.T) {
	s := worksServer(t)
	gateway := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	}))
	gateway.EnableHTTP2 = true
	gateway.StartTLS()
	defer gateway.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if configured, ok := s.reviewHTTP.Transport.(*http.Transport); ok {
		transport = configured.Clone()
	}
	// Keep the application's ALPN list; only trust this isolated TLS fixture.
	transport.TLSClientConfig.RootCAs = gateway.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	s.reviewHTTP.Transport = transport
	if _, err := s.postBytesWithClient(context.Background(), s.reviewHTTP, gateway.URL, map[string]string{"model": "test"}, "test"); err != nil {
		t.Fatalf("model gateway protocol incompatibility: %v", err)
	}
}
