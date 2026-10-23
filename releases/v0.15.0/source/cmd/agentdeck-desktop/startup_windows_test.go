package main

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

type fakeStartupValue struct {
	value   string
	err     error
	deleted bool
}

func TestActivationNotification(t *testing.T) {
	name := fmt.Sprintf(`Local\AgentDeck-Test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	opened := make(chan struct{}, 2)
	cleanup, err := listenForActivation(name, func() { opened <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		notifyActivation(name)
		select {
		case <-opened:
		case <-time.After(2 * time.Second):
			t.Fatal("existing instance was not notified")
		}
	}
	cleanup()
	notifyActivation(name) // A missing listener must be harmless.
}

func (f *fakeStartupValue) read() (string, error) { return f.value, f.err }
func (f *fakeStartupValue) write(v string) error {
	if f.err != nil {
		return f.err
	}
	f.value = v
	return nil
}
func (f *fakeStartupValue) remove() error {
	if f.err != nil {
		return f.err
	}
	f.deleted = true
	f.value = ""
	return nil
}

func TestStartupRegistration(t *testing.T) {
	f := &fakeStartupValue{}
	r := startupRegistration{value: f, executable: func() (string, error) { return `D:\应用 程序\AgentDeck.exe`, nil }}
	if on, err := r.enabled(); err != nil || on {
		t.Fatalf("default: %v %v", on, err)
	}
	if err := r.set(true); err != nil {
		t.Fatal(err)
	}
	if f.value != `"D:\应用 程序\AgentDeck.exe" --startup` {
		t.Fatalf("unsafe command %q", f.value)
	}
	if on, err := r.enabled(); err != nil || !on {
		t.Fatalf("enabled: %v %v", on, err)
	}
	if err := r.set(false); err != nil || !f.deleted {
		t.Fatalf("disable %v", err)
	}
	f.err = errors.New("access denied")
	if _, err := r.enabled(); err == nil {
		t.Fatal("read failure hidden")
	}
	if err := r.set(true); err == nil {
		t.Fatal("write failure hidden")
	}
	f.err = nil
	r.executable = func() (string, error) { return "", errors.New("missing exe") }
	if err := r.set(true); err == nil {
		t.Fatal("missing executable accepted")
	}
	if err := r.set(false); err != nil {
		t.Fatal("disable should not need executable", err)
	}
}
