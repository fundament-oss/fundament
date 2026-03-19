package errors

import (
	"fmt"
	"testing"
)

func TestTransient(t *testing.T) {
	orig := fmt.Errorf("connection refused")
	err := NewTransient(orig)

	if !IsTransient(err) {
		t.Fatal("expected IsTransient to return true")
	}
	if IsPermanent(err) {
		t.Fatal("expected IsPermanent to return false for transient error")
	}
	if err.Error() != "connection refused" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestPermanent(t *testing.T) {
	orig := fmt.Errorf("invalid license")
	err := NewPermanent(orig)

	if !IsPermanent(err) {
		t.Fatal("expected IsPermanent to return true")
	}
	if IsTransient(err) {
		t.Fatal("expected IsTransient to return false for permanent error")
	}
	if err.Error() != "invalid license" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestTransientUnwrap(t *testing.T) {
	inner := fmt.Errorf("timeout")
	err := NewTransient(inner)

	wrapped := fmt.Errorf("outer: %w", err)
	if !IsTransient(wrapped) {
		t.Fatal("expected IsTransient to find transient error in chain")
	}
}

func TestPermanentUnwrap(t *testing.T) {
	inner := fmt.Errorf("bad config")
	err := NewPermanent(inner)

	wrapped := fmt.Errorf("outer: %w", err)
	if !IsPermanent(wrapped) {
		t.Fatal("expected IsPermanent to find permanent error in chain")
	}
}

func TestUntypedError(t *testing.T) {
	err := fmt.Errorf("some error")
	if IsTransient(err) {
		t.Fatal("plain error should not be transient")
	}
	if IsPermanent(err) {
		t.Fatal("plain error should not be permanent")
	}
}

func TestNilError(t *testing.T) {
	if IsTransient(nil) {
		t.Fatal("nil should not be transient")
	}
	if IsPermanent(nil) {
		t.Fatal("nil should not be permanent")
	}
}
