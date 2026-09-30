package probe

import "testing"

func TestThreshold(t *testing.T) {
	thr := Threshold{
		LossPct:    1,
		RTTFactor:  3,
		RTTFloorMs: 50,
		MinSamples: 5,
	}

	if !thr.Tripped(1, 0, false, 0, 0) {
		t.Fatal("loss at threshold should trip")
	}
	if thr.Tripped(0.9, 500, true, 10, 4) {
		t.Fatal("rtt rule needs 5 samples")
	}
	if thr.Tripped(0, 40, true, 10, 5) {
		t.Fatal("below floor")
	}
	if !thr.Tripped(0, 80, true, 10, 5) {
		t.Fatal("above floor")
	}
	if thr.Tripped(0, 300, true, 100, 5) {
		t.Fatal("equal to 3x median")
	}
	if !thr.Tripped(0, 301, true, 100, 5) {
		t.Fatal("above 3x median")
	}
}

func TestMTRTarget(t *testing.T) {
	samples := []Sample{
		{Target: "192.168.0.1", Kind: KindGateway, Loss: 50, Incident: true},
		{Target: "1.1.1.1", Kind: KindExternal, Loss: 1, RTT: 20, Incident: true},
		{Target: "8.8.8.8", Kind: KindExternal, Loss: 10, RTT: 30, Incident: true},
		{Target: "9.9.9.9", Kind: KindExternal, Loss: 10, RTT: 90, Incident: true},
	}
	got, ok := MTRTarget(samples, "1.1.1.1")
	if !ok || got != "9.9.9.9" {
		t.Fatalf("worst = %s %v", got, ok)
	}

	gatewayOnly := []Sample{
		{Target: "192.168.0.1", Kind: KindGateway, Loss: 20, Incident: true},
		{Target: "1.1.1.1", Kind: KindExternal, Loss: 0, Incident: false},
	}
	got, ok = MTRTarget(gatewayOnly, "1.1.1.1")
	if !ok || got != "1.1.1.1" {
		t.Fatalf("fallback = %s %v", got, ok)
	}

	if _, ok := MTRTarget([]Sample{{Target: "1.1.1.1", Kind: KindExternal}}, "1.1.1.1"); ok {
		t.Fatal("quiet sample should not pick a target")
	}
}
