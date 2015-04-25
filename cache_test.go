package main

import "testing"

func TestCache(t *testing.T) {
	c := newCache(5)
	if c == nil {
		t.Fatal("newCache returns nil")
	}
	// basic
	if !c.AddTry("foo") {
		t.Error(`can't add first "foo"`)
	}
	if c.AddTry("foo") {
		t.Error(`second "foo" must not be added`)
	}
	// add more items
	if !c.AddTry("bar") {
		t.Error(`can't add first "bar"`)
	}
	if !c.AddTry("baz") {
		t.Error(`can't add first "baz"`)
	}
	if c.AddTry("bar") {
		t.Error(`second "foo" must not be added`)
	}
	if c.AddTry("baz") {
		t.Error(`second "foo" must not be added`)
	}
	if c.AddTry("foo") {
		t.Error(`third "foo" must not be added`)
	}
	// capacity
	if !c.AddTry("qux") {
		t.Error(`can't add first "qux"`)
	}
	if c.AddTry("foo") {
		t.Error(`4th "foo" must not be added`)
	}
	if !c.AddTry("quux") {
		t.Error(`can't add first "quux"`)
	}
	if !c.AddTry("corge") {
		t.Error(`can't add first "corge"`)
	}
	if !c.AddTry("foo") {
		t.Error(`can't add 5th "foo"`)
	}
	if c.AddTry("baz") {
		t.Error(`second "baz" must not be added`)
	}
	// delete
	c.Delete("baz")
	if !c.AddTry("baz") {
		t.Error(`can't add third "baz"`)
	}
}
