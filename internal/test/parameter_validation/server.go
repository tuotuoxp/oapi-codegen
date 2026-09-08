package parametervalidation

import (
	"net/http"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=config.yaml spec.yaml

type CustomCode string

func (c *CustomCode) UnmarshalText(text []byte) error {
	if len(text) == 0 || string(text) == "bad" {
		return &customCodeError{}
	}
	*c = CustomCode(text)
	return nil
}

type customCodeError struct{}

func (e *customCodeError) Error() string {
	return "invalid custom code"
}

type fakeServer struct {
	called bool
}

func (s *fakeServer) GetValidatedString(w http.ResponseWriter, r *http.Request, params GetValidatedStringParams) {
	s.called = true
	w.WriteHeader(http.StatusNoContent)
}

func (s *fakeServer) GetValidatedNumber(w http.ResponseWriter, r *http.Request, id GetValidatedNumberParamsId) {
	s.called = true
	w.WriteHeader(http.StatusNoContent)
}

func (s *fakeServer) GetCustom(w http.ResponseWriter, r *http.Request, params GetCustomParams) {
	s.called = true
	w.WriteHeader(http.StatusNoContent)
}
