package runtime

import (
	"bytes"
	"compress/gzip"
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
}

func (w *compressResponseWriter) Write(data []byte) (int, error) {
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
	case w.buffer.Len() > 0:
		w.ResponseWriter.WriteHeader(w.status)
		w.started = true
		if _, err := w.ResponseWriter.Write(w.buffer.Bytes()); err != nil {
			return
		}
	case !w.started:
		w.ResponseWriter.WriteHeader(w.status)
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

func requestAcceptsGzip(r *http.Request) bool {
	return collectionlist.NewList(strings.Split(r.Header.Get("Accept-Encoding"), ",")...).
		AnyMatch(func(_ int, encoding string) bool {
			name, _, _ := strings.Cut(strings.TrimSpace(encoding), ";")
			return strings.EqualFold(strings.TrimSpace(name), "gzip")
		})
}
