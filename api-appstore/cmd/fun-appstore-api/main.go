package main

import (
	"log"
	"time"

	"k8s.io/utils/ptr"
)

func main() {
	// Remove this later, I needed an external dependency so that go creates
	// `go.sum` so that the Dockerfiles will work. Interesting fact: in go 1.26
	// (rc1 just released) the behavior of ptr.To is added as feature to the
	// native `new()` func.
	var x *int
	x = ptr.To(24)

	log.Printf("appstore-api started %d\n", *x)
	for {
		time.Sleep(time.Hour)
	}
}
