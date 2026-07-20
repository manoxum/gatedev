package discover

import (
	"testing"

	"bindnet/dns-provider/internal/core"
)

func TestMergeAdvertisedManualRemoteModeSkipsIndirectRoutes(t *testing.T) {
	cfg := &core.Config{NodeName: "local", RemoteMode: "manual"}
	updated := map[string]core.Route{}

	mergeAdvertised(cfg, updated, "10.0.0.2:8531", []advertisedRoute{
		{Domain: "direct.bnet", Owner: "direct", Distance: 0},
		{Domain: "remote.bnet", Owner: "remote", Distance: 1},
	})

	if _, ok := updated["direct.bnet"]; !ok {
		t.Fatalf("direct route was not learned")
	}
	if _, ok := updated["remote.bnet"]; ok {
		t.Fatalf("indirect route was learned while DISCOVER_REMOTE_ROUTES=manual")
	}
}

func TestMergeAdvertisedAutoRemoteModeLearnsIndirectRoutes(t *testing.T) {
	cfg := &core.Config{NodeName: "local", RemoteMode: "auto"}
	updated := map[string]core.Route{}

	mergeAdvertised(cfg, updated, "10.0.0.2:8531", []advertisedRoute{
		{Domain: "remote.bnet", Owner: "remote", Distance: 1},
	})

	if _, ok := updated["remote.bnet"]; !ok {
		t.Fatalf("indirect route was not learned while DISCOVER_REMOTE_ROUTES=auto")
	}
}

func TestMergeAdvertisedSkipsOwnFingerprint(t *testing.T) {
	cfg := &core.Config{NodeName: "Daniel Costa", Fingerprint: "local-fp", RemoteMode: "auto"}
	updated := map[string]core.Route{}

	mergeAdvertised(cfg, updated, "10.0.0.2:8531", []advertisedRoute{
		{Domain: "costa.bnet", Owner: "Daniel Costa", OwnerFingerprint: "local-fp", Distance: 1},
	})

	if _, ok := updated["costa.bnet"]; ok {
		t.Fatalf("own fingerprint was learned as a remote route")
	}
}
