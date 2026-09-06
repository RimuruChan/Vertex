//go:build !linux

package instance_test

import (
	"testing"

	"github.com/RimuruChan/Vertex/worker/internal/instance"
)

func TestAcquireFailsClosedOffLinux(t *testing.T) {
	if _, err := instance.Acquire("unused"); err == nil {
		t.Fatal("Acquire succeeded without Linux flock support")
	}
}
