package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/samber/oops"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, oops.In("runtime").Wrapf(err, "hijack response writer")
	}
	return conn, rw, nil
}

func (r *statusRecorder) ReadFrom(reader io.Reader) (int64, error) {
	if readerFrom, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(reader)
		if err != nil {
			return n, oops.In("runtime").Wrapf(err, "read response body from upstream")
		}
		return n, nil
	}
	n, err := io.Copy(r.ResponseWriter, reader)
	if err != nil {
		return n, oops.In("runtime").Wrapf(err, "copy response body from upstream")
	}
	return n, nil
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return fmt.Errorf("push response: %w", http.ErrNotSupported)
	}
	if err := pusher.Push(target, opts); err != nil {
		return oops.In("runtime").With("target", target).Wrapf(err, "push response")
	}
	return nil
}
