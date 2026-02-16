package clock

import (
	"time"
)

type Clock interface {
	Now() time.Time
	UpdateTime(t time.Time)
}

type realClock struct{}

func New() Clock {
	return &realClock{}
}

func (c *realClock) Now() time.Time {
	return time.Now()
}

func (c *realClock) UpdateTime(time.Time) {}

type testClock struct {
	now time.Time
}

func NewTest(t time.Time) Clock {
	return &testClock{now: t}
}

func (c *testClock) Now() time.Time {
	return c.now
}

func (c *testClock) UpdateTime(t time.Time) {
	c.now = t
}
