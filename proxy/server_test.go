package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestStartReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	server := NewServer()
	defer server.Close()

	if err := server.Start(port); err == nil {
		t.Fatalf("Start(%d) succeeded although the port was already occupied", port)
	}
	if server.IsRunning() {
		t.Fatal("server reported running after a bind failure")
	}
}

func TestServerCanRenderAfterStopAndRestart(t *testing.T) {
	server := NewServer()
	defer server.Close()

	if err := server.Start(0); err != nil {
		t.Fatal(err)
	}
	firstPool := server.rendererPool
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	if !firstPool.IsClosed() {
		t.Fatal("renderer pool was not closed on stop")
	}

	if err := server.Start(0); err != nil {
		t.Fatal(err)
	}
	if server.rendererPool == firstPool {
		t.Fatal("closed renderer pool was reused after restart")
	}
	if server.rendererPool.IsClosed() {
		t.Fatal("replacement renderer pool is closed")
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLModesThroughRunningProxy(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chromium-based browser is not installed")
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Mode probe</title></head><body><h1 id="probe">RetroProxy mode probe</h1><p>Visible mode content</p><script>document.body.setAttribute("data-rendered", "yes")</script></body></html>`)
	}))
	defer origin.Close()

	server := NewServer()
	server.SetLogger(func(string) {})
	defer server.Close()
	if err := server.Start(0); err != nil {
		t.Fatal(err)
	}
	// Exercise the same lifecycle as the UI: stop, then start again, before
	// verifying every mode. This guards against reusing a closed renderer pool.
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(0); err != nil {
		t.Fatal(err)
	}

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", server.GetPort()))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			DisableKeepAlives:   true,
			MaxIdleConnsPerHost: -1,
		},
	}

	tests := []struct {
		mode        string
		want        string
		mustNotHave string
	}{
		{mode: "modern", want: "RetroProxy mode probe"},
		{mode: "3.2", want: "HTML 3.2 Final"},
		{mode: "3.2new", want: "HTML 3.2 Final"},
		{mode: "4.01", want: "HTML 4.01 Transitional"},
		{mode: "text", want: "Text Mode", mustNotHave: "<script"},
		{mode: "image", want: "usemap="},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			server.SetHTMLVersion(tt.mode)
			resp, err := client.Get(origin.URL)
			if err != nil {
				t.Fatalf("proxy request failed: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body = %.500s", resp.StatusCode, body)
			}
			if !strings.Contains(string(body), tt.want) {
				t.Fatalf("response does not contain %q: %.500s", tt.want, body)
			}
			if tt.mustNotHave != "" && strings.Contains(strings.ToLower(string(body)), tt.mustNotHave) {
				t.Fatalf("response unexpectedly contains %q", tt.mustNotHave)
			}
		})
	}
}
