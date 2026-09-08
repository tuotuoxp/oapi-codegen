package parametervalidation

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationFailureUsesBindErrorCallback(t *testing.T) {
	server := &fakeServer{}

	var callbackErr error
	h := HandlerWithOptions(server, ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			callbackErr = err
			w.WriteHeader(http.StatusBadRequest)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/validated-string?name=ab", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Error(t, callbackErr)
	assert.False(t, server.called)
	var invalid *InvalidParamFormatError
	assert.True(t, errors.As(callbackErr, &invalid))
}

func TestValidationSuccessPassesThrough(t *testing.T) {
	server := &fakeServer{}

	h := Handler(server)
	req := httptest.NewRequest(http.MethodGet, "/validated-string?name=abcd", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.True(t, server.called)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestNumericValidationFailureUsesBindErrorCallback(t *testing.T) {
	server := &fakeServer{}

	var callbackErr error
	h := HandlerWithOptions(server, ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			callbackErr = err
			w.WriteHeader(http.StatusBadRequest)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/validated-number/2", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Error(t, callbackErr)
	assert.False(t, server.called)
	var invalid *InvalidParamFormatError
	assert.True(t, errors.As(callbackErr, &invalid))
}

func TestXGoTypeFailureUsesBindErrorCallback(t *testing.T) {
	server := &fakeServer{}

	var callbackErr error
	h := HandlerWithOptions(server, ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			callbackErr = err
			w.WriteHeader(http.StatusBadRequest)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/custom?code=bad", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Error(t, callbackErr)
	assert.False(t, server.called)
	var invalid *InvalidParamFormatError
	assert.True(t, errors.As(callbackErr, &invalid))
}

func TestXGoTypeSuccessPassesThrough(t *testing.T) {
	server := &fakeServer{}

	h := Handler(server)
	req := httptest.NewRequest(http.MethodGet, "/custom?code=GOOD", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.True(t, server.called)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}
