package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// cloudEventData extracts the `data` field from a CloudEvent structured body.
// Returns the raw data bytes, or nil if the request is not a CloudEvent.
// Knative RabbitmqSource sends Content-Type: application/cloudevents+json.
func cloudEventData(r *http.Request) ([]byte, error) {
	ct := r.Header.Get("Content-Type")
	if !strings.Contains(ct, "cloudevents") {
		return nil, nil // not a CloudEvent — caller reads body directly
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var ce struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &ce); err != nil {
		return nil, err
	}
	// json.RawMessage for a JSON null value is []byte("null"), not Go nil.
	if ce.Data == nil || string(ce.Data) == "null" {
		return nil, nil
	}
	return ce.Data, nil
}
