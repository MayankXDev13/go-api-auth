package ratelimit

import "testing"

func TestMemory_AllowsBurstThenDenies(t *testing.T) {
	l := NewMemory(3)

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th request should be denied")
	}
	if !l.Allow("other") {
		t.Fatal("a different key should be allowed")
	}
}
