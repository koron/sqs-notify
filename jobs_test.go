package main

import "testing"

func jobAssertStart(t *testing.T, j jobs, id string, n int) {
	s := j.StartTry(id)
	if s != jobStarted {
		t.Errorf(`couldn't start job:%s(%d) with %#v`, id, n, s)
	}
}

func jobAssertNotStart(t *testing.T, j jobs, id string, n int) {
	s := j.StartTry(id)
	if s == jobStarted {
		t.Errorf(`job:%s(%d) must not be started`, id, n)
	}
}

func TestJobs(t *testing.T) {
	j, err := newJobs(5)
	if err != nil {
		t.Fatalf("newJobs must not return error: %s", err)
	}
	if j == nil {
		t.Fatal("newJobs must not return nil")
	}

	// basic
	jobAssertStart(t, j, "foo", 1)
	jobAssertNotStart(t, j, "foo", 2)

	// add more jobs
	jobAssertStart(t, j, "bar", 1)
	jobAssertStart(t, j, "baz", 1)
	jobAssertNotStart(t, j, "bar", 2)
	jobAssertNotStart(t, j, "baz", 2)
	jobAssertNotStart(t, j, "foo", 3)

	// capacity
	jobAssertStart(t, j, "qux", 1)
	jobAssertNotStart(t, j, "foo", 4)
	jobAssertStart(t, j, "quux", 1)
	jobAssertStart(t, j, "corge", 1)
	jobAssertStart(t, j, "foo", 5)
	jobAssertNotStart(t, j, "baz", 3)
	jobAssertNotStart(t, j, "qux", 2)
	jobAssertNotStart(t, j, "quux", 2)
	jobAssertNotStart(t, j, "corge", 2)
	jobAssertNotStart(t, j, "foo", 6)

	// fail
	j.Fail("baz")
	jobAssertStart(t, j, "baz", 4)

	// complete
	j.Complete("corge")
	if s := j.StartTry("corge"); s != jobCompleted {
		t.Errorf(`"corge" must not started with %d, but %d`, jobCompleted, s)
	}
	if s := j.StartTry("foo"); s != jobRunning {
		t.Errorf(`"corge" must not started with %d, but %d`, jobRunning, s)
	}
}
