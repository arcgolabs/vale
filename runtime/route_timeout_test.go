package runtime_test

import (
	"net/http"
	"testing"
	"time"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestRouteWriteTimeoutBuilderAndAdminView(t *testing.T) {
	t.Parallel()

	timeout := 250 * time.Millisecond
	route := valeruntime.NewRoute("events", "web", nil).WithWriteTimeout(timeout)
	if route.WriteTimeout == nil || *route.WriteTimeout != timeout {
		t.Fatalf("route write timeout = %v, want %v", route.WriteTimeout, timeout)
	}
	snapshot := valeruntime.NewSnapshot().AddRoute(route).BuildCatalog()
	views := snapshot.Routes()
	view, ok := views.Get(0)
	if !ok || view.WriteTimeout != "250ms" {
		t.Fatalf("route view = %#v, want write_timeout 250ms", view)
	}
}

func TestGatewayAppliesRouteWriteTimeoutAndDisablesItForSSE(t *testing.T) {
	t.Parallel()

	writer := &deadlineResponseWriter{header: make(http.Header)}
	endpoint, err := valeruntime.NewEndpoint("http://127.0.0.1:8081", 1, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, writeErr := w.Write([]byte("data: [DONE]\n\n")); writeErr != nil {
			t.Error(writeErr)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("events", "round_robin", endpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(service).
		AddRoute(valeruntime.NewRoute("events", "web", service).WithWriteTimeout(time.Second)).
		BuildMatchers()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	valeruntime.NewGateway(snapshot, nil, false, valeruntime.NewNoopMetrics()).Handler("web").ServeHTTP(writer, request)

	if len(writer.deadlines) != 2 {
		t.Fatalf("write deadlines = %v, want route deadline followed by SSE disable", writer.deadlines)
	}
	if writer.deadlines[0].IsZero() || !writer.deadlines[1].IsZero() {
		t.Fatalf("write deadlines = %v, want non-zero then zero", writer.deadlines)
	}
	if string(writer.body) != "data: [DONE]\n\n" {
		t.Fatalf("body = %q", writer.body)
	}
}

type deadlineResponseWriter struct {
	header    http.Header
	body      []byte
	status    int
	deadlines []time.Time
}

func (w *deadlineResponseWriter) Header() http.Header { return w.header }

func (w *deadlineResponseWriter) WriteHeader(status int) { w.status = status }

func (w *deadlineResponseWriter) Write(data []byte) (int, error) {
	w.body = append(w.body, data...)
	return len(data), nil
}

func (w *deadlineResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (w *deadlineResponseWriter) FlushError() error { return nil }
