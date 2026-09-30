package probe

import (
	"strings"
	"testing"
)

func TestParseDefaultRoute(t *testing.T) {
	in := `
Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
eth0	0000A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
eth1	00000000	FE01A8C0	0003	0	0	100	00000000	0	0	0
eth0	00000000	0100A8C0	0003	0	0	50	00000000	0	0	0
`
	iface, gw, err := ParseDefaultRoute(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if iface != "eth0" || gw != "192.168.0.1" {
		t.Fatalf("route = %s %s", iface, gw)
	}
}

func TestParseDefaultRouteMissing(t *testing.T) {
	in := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n"
	if _, _, err := ParseDefaultRoute(strings.NewReader(in)); err == nil {
		t.Fatal("expected error")
	}
}
