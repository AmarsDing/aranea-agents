package runtime

import "testing"

func TestMemorySetAvailable(t *testing.T) {
	if (MemorySet{}).Available() {
		t.Fatal("empty set should not be available")
	}
}
