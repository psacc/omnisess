package report

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleResult()); err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
	}
	if _, ok := got["skills"]; !ok {
		t.Errorf("expected skills key in output: %v", got)
	}
	if _, ok := got["metadata"]; !ok {
		t.Errorf("expected metadata key in output: %v", got)
	}
}

func TestJSONIndented(t *testing.T) {
	var buf bytes.Buffer
	JSON(&buf, sampleResult())
	if !bytes.Contains(buf.Bytes(), []byte("\n  ")) {
		t.Errorf("expected indented JSON output")
	}
}
