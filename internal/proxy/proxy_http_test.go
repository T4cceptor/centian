package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/assert"
)

func TestDelayedResponseWriterWriteHeaderAndFlush(t *testing.T) {
	writer := newDelayedResponseWriter()

	writer.WriteHeader(http.StatusAccepted)
	writer.Flush()
	_, err := writer.Write([]byte("ok"))

	assert.NilError(t, err)
	assert.Equal(t, writer.statusCode, http.StatusAccepted)
	assert.Equal(t, writer.body.String(), "ok")
}

func TestWriteDelayedResponseCopiesBufferedResponse(t *testing.T) {
	source := newDelayedResponseWriter()
	source.Header().Add("X-Test", "value")
	source.WriteHeader(http.StatusCreated)
	_, err := source.Write([]byte("payload"))
	assert.NilError(t, err)

	target := httptest.NewRecorder()
	writeDelayedResponse(target, source)

	assert.Equal(t, target.Code, http.StatusCreated)
	assert.Equal(t, target.Header().Get("X-Test"), "value")
	assert.Equal(t, target.Body.String(), "payload")
}
