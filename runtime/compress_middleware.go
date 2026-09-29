package runtime

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

func wrapCompressMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.Compress.Enabled {
		return next
	}
	minBytes := middleware.Compress.MinBytes
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestAcceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		writer := newCompressResponseWriter(w, minBytes)
		defer writer.finish()
		next.ServeHTTP(writer, r)
	})
}

type compressResponseWriter struct {
	http.ResponseWriter
	status     int
	minBytes   int
	buffer     bytes.Buffer
	gzipWriter *gzip.Writer
	started    bool
	plain      bool
	writeErr   error
}

func newCompressResponseWriter(w http.ResponseWriter, minBytes int) *compressResponseWriter {
	return &compressResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
		minBytes:       minBytes,
	}
}

func (w *compressResponseWriter) WriteHeader(statusCode int) {
	if w.started {
		return
	}
	w.status = statusCode
	if isEventStreamContentType(w.Header().Get("Content-Type")) {
		w.writeErr = w.startPlain()
	}
}

func (w *compressResponseWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.plain {
		return w.writePlain(data)
	}
	if isEventStreamContentType(w.Header().Get("Content-Type")) {
		if err := w.startPlain(); err != nil {
			return 0, err
		}
		return w.writePlain(data)
	}
	if w.gzipWriter != nil {
		return w.writeGzip(data)
	}
	if w.shouldBuffer(data) {
		return w.writePlainBuffer(data)
	}
	w.startGzip()
	if err := w.flushBufferToGzip(); err != nil {
		return 0, err
	}
	return w.writeGzip(data)
}

func (w *compressResponseWriter) finish() {
	switch {
	case w.gzipWriter != nil:
		if err := w.gzipWriter.Close(); err != nil {
			return
		}
	case w.plain:
		return
	default:
		if err := w.startPlain(); err != nil {
			return
		}
	}
}

func (w *compressResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compressResponseWriter) Flush() {
	switch {
	case w.gzipWriter != nil:
		if err := w.gzipWriter.Flush(); err != nil {
			w.writeErr = oops.In("runtime").With("middleware", "compress").Wrapf(err, "flush gzip response")
			return
		}
	case !w.plain:
		if err := w.startPlain(); err != nil {
			w.writeErr = err
			return
		}
	}
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		return
	}
}

func (w *compressResponseWriter) shouldBuffer(data []byte) bool {
	return w.minBytes > 0 && w.buffer.Len()+len(data) < w.minBytes
}

func (w *compressResponseWriter) writePlainBuffer(data []byte) (int, error) {
	if _, err := w.buffer.Write(data); err != nil {
		return 0, oops.In("runtime").With("middleware", "compress").Wrapf(err, "buffer response")
	}
	return len(data), nil
}

func (w *compressResponseWriter) writePlain(data []byte) (int, error) {
	written, err := w.ResponseWriter.Write(data)
	if err != nil {
		return written, oops.In("runtime").With("middleware", "compress").Wrapf(err, "write plain response")
	}
	return written, nil
}

func (w *compressResponseWriter) writeGzip(data []byte) (int, error) {
	if _, err := w.gzipWriter.Write(data); err != nil {
		return 0, oops.In("runtime").With("middleware", "compress").Wrapf(err, "write gzip response")
	}
	return len(data), nil
}

func (w *compressResponseWriter) flushBufferToGzip() error {
	if w.buffer.Len() == 0 {
		return nil
	}
	if _, err := w.gzipWriter.Write(w.buffer.Bytes()); err != nil {
		return oops.In("runtime").With("middleware", "compress").Wrapf(err, "write buffered gzip response")
	}
	w.buffer.Reset()
	return nil
}

func (w *compressResponseWriter) startGzip() {
	if w.started {
		return
	}
	header := w.Header()
	header.Add("Vary", "Accept-Encoding")
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	w.ResponseWriter.WriteHeader(w.status)
	w.gzipWriter = gzip.NewWriter(w.ResponseWriter)
	w.started = true
}

func (w *compressResponseWriter) startPlain() error {
	if w.started {
		return nil
	}
	w.ResponseWriter.WriteHeader(w.status)
	w.started = true
	w.plain = true
	if w.buffer.Len() == 0 {
		return nil
	}
	if _, err := io.Copy(w.ResponseWriter, &w.buffer); err != nil {
		return oops.In("runtime").With("middleware", "compress").Wrapf(err, "write buffered response")
	}
	return nil
}

func requestAcceptsGzip(r *http.Request) bool {
	return collectionlist.NewList(strings.Split(r.Header.Get("Accept-Encoding"), ",")...).
		Stream().Any(func(encoding string) bool {
		name, _, _ := strings.Cut(strings.TrimSpace(encoding), ";")
		return strings.EqualFold(strings.TrimSpace(name), "gzip")
	})
}
