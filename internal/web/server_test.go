package web_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/web"
)

// Binds a local port. Inside the Claude Code sandbox this needs
// sandbox.network.allowLocalBinding: true; otherwise run from a normal terminal.
func TestServeFinishesInFlightRequestOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "done")
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var serveErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		serveErr = web.Serve(ctx, srv, ln, 5*time.Second)
	}()

	var (
		status  int
		body    []byte
		respErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow") //nolint:noctx // test client
		if err != nil {
			respErr = err
			return
		}
		defer func() { _ = resp.Body.Close() }()
		status = resp.StatusCode
		body, respErr = io.ReadAll(resp.Body)
	}()

	<-started
	cancel() // simulate SIGTERM while /slow is in flight
	wg.Wait()

	require.NoError(t, serveErr)
	require.NoError(t, respErr)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "done", string(body))
}

func TestServeReturnsListenerErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_ = ln.Close() // a closed listener makes Serve fail immediately

	err = web.Serve(context.Background(), &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}, ln, time.Second)
	require.Error(t, err)
}
