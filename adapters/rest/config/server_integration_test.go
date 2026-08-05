//go:build integration

package config

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGracefulShutdownDrainsSlowRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	engine.GET("/slow", func(c *gin.Context) {
		close(requestStarted)
		<-releaseRequest
		c.Status(http.StatusNoContent)
	})

	cfg := &Config{App: AppConfig{Port: 0, ShutdownTimeout: 2 * time.Second}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }
	t.Cleanup(release)
	serverStarted := make(chan net.Addr, 1)
	runDone := make(chan error, 1)
	go func() { runDone <- run(ctx, cfg, engine, serverStarted) }()

	var address net.Addr
	select {
	case address = <-serverStarted:
	case err := <-runDone:
		t.Fatalf("server did not start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start")
	}
	_, port, err := net.SplitHostPort(address.String())
	if err != nil {
		t.Fatal(err)
	}
	responseDone := make(chan error, 1)
	go func() {
		response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/slow", port))
		if err == nil {
			defer response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				err = fmt.Errorf("status = %d", response.StatusCode)
			}
		}
		responseDone <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach handler")
	}
	cancel()
	select {
	case err := <-runDone:
		t.Fatalf("server returned before in-flight request completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()

	select {
	case err := <-responseDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
}
