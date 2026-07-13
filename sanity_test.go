package main

import (
	"strings"
	"testing"
)

func TestTailerWritesToCallback(t *testing.T) {
	var got string
	tailer := &tailer{
		OnWrite: func(s string) {
			got = s
		},
	}

	n, err := tailer.Write([]byte("stargate ready"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len("stargate ready") {
		t.Fatalf("Write returned %d bytes, want %d", n, len("stargate ready"))
	}
	if !strings.Contains(got, "ready") {
		t.Fatalf("callback received %q, want ready marker", got)
	}
}
