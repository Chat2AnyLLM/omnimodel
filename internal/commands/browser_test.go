package commands

import "testing"

func TestTryOpenBrowserCallsInjectedFunc(t *testing.T) {
	called := false
	var gotURL string
	orig := openBrowserFunc
	openBrowserFunc = func(url string) bool {
		called = true
		gotURL = url
		return true
	}
	defer func() { openBrowserFunc = orig }()

	result := tryOpenBrowser("https://github.com/login/device")

	if !called {
		t.Fatal("expected openBrowserFunc to be called")
	}
	if gotURL != "https://github.com/login/device" {
		t.Fatalf("expected URL %q, got %q", "https://github.com/login/device", gotURL)
	}
	if !result {
		t.Fatal("expected tryOpenBrowser to return true")
	}
}

func TestTryOpenBrowserReturnsFalseOnFailure(t *testing.T) {
	orig := openBrowserFunc
	openBrowserFunc = func(_ string) bool { return false }
	defer func() { openBrowserFunc = orig }()

	if tryOpenBrowser("https://example.com") {
		t.Fatal("expected tryOpenBrowser to return false when browser fails to open")
	}
}

func TestTryOpenBrowserPropagatesURL(t *testing.T) {
	const target = "https://copilot.github.com/device"
	var captured string
	orig := openBrowserFunc
	openBrowserFunc = func(url string) bool {
		captured = url
		return true
	}
	defer func() { openBrowserFunc = orig }()

	tryOpenBrowser(target)

	if captured != target {
		t.Fatalf("expected URL %q, got %q", target, captured)
	}
}
