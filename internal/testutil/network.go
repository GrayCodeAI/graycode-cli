package testutil

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/netutil"
)

const (
	LoopbackHost        = netutil.LoopbackHost
	LoopbackDynamicAddr = netutil.LoopbackDynamicAddr
	LoopbackNoProxy     = netutil.LoopbackNoProxy
)

func IsLoopbackUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied")
}

func SkipIfLoopbackUnavailable(t testing.TB, err error) {
	t.Helper()
	if IsLoopbackUnavailable(err) {
		t.Skipf("loopback listener unavailable: %v", err)
	}
}

func ListenLoopback(t testing.TB) net.Listener {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", LoopbackDynamicAddr)
	if err != nil {
		SkipIfLoopbackUnavailable(t, err)
		t.Fatalf("listen loopback: %v", err)
	}
	return ln
}

func NewLoopbackHTTPServer(t testing.TB, handler http.Handler) *httptest.Server {
	t.Helper()
	ts := &httptest.Server{
		Listener: ListenLoopback(t),
		Config:   &http.Server{Handler: handler},
	}
	ts.Start()
	return ts
}
