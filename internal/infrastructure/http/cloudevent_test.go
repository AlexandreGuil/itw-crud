package http

import (
	"bytes"
	"net/http"
	"testing"
)

func TestCloudEventData_ExtractsDataFromStructured(t *testing.T) {
	body := []byte(`{"specversion":"1.0","type":"dev.knative.rabbitmq.event","data":{"url":"https://example.com","md5_url":"abc123"}}`)
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")

	data, err := cloudEventData(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if string(data) != `{"url":"https://example.com","md5_url":"abc123"}` {
		t.Errorf("unexpected data: %s", data)
	}
}

func TestCloudEventData_PassthroughForPlainJSON(t *testing.T) {
	body := []byte(`{"url":"https://example.com"}`)
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	data, err := cloudEventData(req)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("expected nil for non-CloudEvent, got: %s", data)
	}
}

func TestCloudEventData_InvalidJSON_ReturnsError(t *testing.T) {
	body := []byte(`not valid json`)
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")

	_, err := cloudEventData(req)
	if err == nil {
		t.Fatal("expected error for invalid JSON cloudevent body")
	}
}

func TestCloudEventData_NullDataField_ReturnsNil(t *testing.T) {
	body := []byte(`{"specversion":"1.0","type":"dev.knative.rabbitmq.event","data":null}`)
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")

	data, err := cloudEventData(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for null data field, got: %s", data)
	}
}
