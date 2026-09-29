package runtime

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/samber/oops"
)

type routeResponseWriter struct {
	http.ResponseWriter
	sseDeadlineDisabled bool
}

func newRouteResponseWriter(w http.ResponseWriter, timeout *time.Duration) *routeResponseWriter {
	writer := &routeResponseWriter{ResponseWriter: w}
	if timeout == nil {
		return writer
	}
	deadline := time.Time{}
	if *timeout > 0 {
		deadline = time.Now().Add(*timeout)
	}
	setWriteDeadlineBestEffort(w, deadline)
	return writer
}

func (w *routeResponseWriter) WriteHeader(statusCode int) {
	w.disableSSEWriteDeadline()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *routeResponseWriter) Write(data []byte) (int, error) {
	w.disableSSEWriteDeadline()
	written, err := w.ResponseWriter.Write(data)
	if err != nil {
		return written, oops.In("runtime").With("component", "route_response").Wrapf(err, "write response")
	}
	return written, nil
}

func (w *routeResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *routeResponseWriter) Flush() {
	w.disableSSEWriteDeadline()
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		return
	}
}

func (w *routeResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	conn, readWriter, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, oops.In("runtime").With("component", "route_response").Wrapf(err, "hijack response")
	}
	return conn, readWriter, nil
}

func (w *routeResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	w.disableSSEWriteDeadline()
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		read, err := readerFrom.ReadFrom(reader)
		if err != nil {
			return read, oops.In("runtime").With("component", "route_response").Wrapf(err, "read response from upstream")
		}
		return read, nil
	}
	copied, err := io.Copy(w.ResponseWriter, reader)
	if err != nil {
		return copied, oops.In("runtime").With("component", "route_response").Wrapf(err, "copy response from upstream")
	}
	return copied, nil
}

func (w *routeResponseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	if err := pusher.Push(target, options); err != nil {
		return oops.In("runtime").With("component", "route_response", "target", target).Wrapf(err, "push response")
	}
	return nil
}

func (w *routeResponseWriter) disableSSEWriteDeadline() {
	if w.sseDeadlineDisabled || !isEventStreamContentType(w.Header().Get("Content-Type")) {
		return
	}
	w.sseDeadlineDisabled = true
	setWriteDeadlineBestEffort(w.ResponseWriter, time.Time{})
}

func setWriteDeadlineBestEffort(writer http.ResponseWriter, deadline time.Time) {
	if err := http.NewResponseController(writer).SetWriteDeadline(deadline); err != nil {
		return
	}
}

func isEventStreamContentType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "text/event-stream")
}
