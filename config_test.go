package main

import (
	"testing"
)

func TestPathReturnsOrdered(t *testing.T) {
	p := []string{
		"1",
		"2",
		"3",
		"4",
	}

	c := sched_config{
		Groups: []*config_group{
			{
				Paths: p,
			},
		},
	}

	for i, e := range p {
		a := c.next_video()
		if a != e {
			t.Fatalf("%d: want %s have %s", i, e, a)
		}
	}

	for i, e := range p {
		a := c.next_video()
		if a != e {
			t.Fatalf("%d (repeat): want %s have %s", i, e, a)
		}
	}
}
