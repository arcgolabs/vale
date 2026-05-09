package raftnode_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arcgolabs/vale/cluster/raftnode"
)

func TestNodeAppliesCertificateCommands(t *testing.T) {
	node := newTestNode(t)
	waitForGroupLeader(t, node, raftnode.CertificatesGroupName)

	resultData, err := node.ProposeGroup(
		raftnode.CertificatesGroupName,
		[]byte(`{"type":"certificate_store","certificate":{"key":"certificates/local/example.com/example.com.crt","value":"Y2VydA=="}}`),
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCommandResultOK(t, resultData)

	state := node.AppliedGroupState(raftnode.CertificatesGroupName)
	if state.Certificates.Len() != 1 {
		t.Fatalf("certificates len = %d, want 1", state.Certificates.Len())
	}
	certificate, _ := state.Certificates.GetFirst()
	if certificate.Key != "certificates/local/example.com/example.com.crt" || string(certificate.Value) != "cert" {
		t.Fatalf("certificate = %#v", certificate)
	}

	mustApplyGroup(t, node, raftnode.CertificatesGroupName, []byte(`{"type":"certificate_delete","certificate":{"key":"certificates/local/example.com"}}`))
	state = node.AppliedGroupState(raftnode.CertificatesGroupName)
	if state.Certificates.Len() != 0 {
		t.Fatalf("certificates len after delete = %d, want 0", state.Certificates.Len())
	}
}

func TestNodeAppliesCertificateLocks(t *testing.T) {
	node := newTestNode(t)
	waitForGroupLeader(t, node, raftnode.CertificatesGroupName)

	first := proposeCertificateLock(t, node, "node-a")
	assertCommandResultOK(t, first)

	second := proposeCertificateLock(t, node, "node-b")
	result := decodeCommandResult(t, second)
	if result.OK || result.Reason != "held" {
		t.Fatalf("second lock result = %#v, want held", result)
	}

	released, err := node.ProposeGroup(
		raftnode.CertificatesGroupName,
		[]byte(`{"type":"certificate_lock_release","lock":{"name":"acme/example","owner":"node-a"}}`),
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCommandResultOK(t, released)
}

func proposeCertificateLock(t *testing.T, node *raftnode.Node, owner string) []byte {
	t.Helper()

	payload := `{"type":"certificate_lock_acquire","lock":{"name":"acme/example","owner":"` + owner + `","requested_at":"2026-05-09T00:00:00Z","expires_at":"2026-05-09T00:10:00Z"}}`
	resultData, err := node.ProposeGroup(raftnode.CertificatesGroupName, []byte(payload), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return resultData
}

func assertCommandResultOK(t *testing.T, data []byte) {
	t.Helper()

	result := decodeCommandResult(t, data)
	if !result.OK {
		t.Fatalf("command result = %#v, want ok", result)
	}
}

func decodeCommandResult(t *testing.T, data []byte) raftnode.CommandResult {
	t.Helper()

	var result raftnode.CommandResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
