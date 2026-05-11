package tools

import "testing"

func TestResolveDockerNetworkDefaultNone(t *testing.T) {
	t.Setenv(sandboxNetworkEnv, "")
	t.Setenv(sandboxAllowlistEnv, "")
	net, err := resolveDockerNetwork()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if net != "none" {
		t.Fatalf("expected default network 'none', got %q", net)
	}
}

func TestResolveDockerNetworkAllowlisted(t *testing.T) {
	t.Setenv(sandboxNetworkEnv, "trusted-net")
	t.Setenv(sandboxAllowlistEnv, "trusted-net,dev-net")
	net, err := resolveDockerNetwork()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if net != "trusted-net" {
		t.Fatalf("expected trusted-net, got %q", net)
	}
}

func TestResolveDockerNetworkRejectedByAllowlist(t *testing.T) {
	t.Setenv(sandboxNetworkEnv, "internet")
	t.Setenv(sandboxAllowlistEnv, "trusted-net,dev-net")
	_, err := resolveDockerNetwork()
	if err == nil {
		t.Fatal("expected allowlist rejection error")
	}
	if err != errSandboxNetworkNotAllowed {
		t.Fatalf("unexpected error type: %v", err)
	}
}
