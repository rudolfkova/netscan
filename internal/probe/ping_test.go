package probe

import "testing"

func TestParsePingOutput(t *testing.T) {
	ok := `
PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.

--- 1.1.1.1 ping statistics ---
10 packets transmitted, 10 received, 0% packet loss, time 1804ms
rtt min/avg/max/mdev = 1.123/1.456/2.789/0.321 ms
`
	stats, err := ParsePingOutput(ok)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sent != 10 || stats.Recv != 10 || stats.LossPct != 0 || !stats.HasRTT {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.Min != 1.123 || stats.Avg != 1.456 || stats.Max != 2.789 || stats.Mdev != 0.321 {
		t.Fatalf("rtt = %+v", stats)
	}

	withErrors := `
--- 8.8.8.8 ping statistics ---
10 packets transmitted, 8 received, +2 errors, 20% packet loss, time 9012ms
rtt min/avg/max/mdev = 10.100/20.200/30.300/4.400 ms
`
	stats, err = ParsePingOutput(withErrors)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sent != 10 || stats.Recv != 8 || stats.LossPct != 20 || stats.Avg != 20.2 {
		t.Fatalf("stats = %+v", stats)
	}

	loss := `
--- 9.9.9.9 ping statistics ---
10 packets transmitted, 0 received, 100% packet loss, time 9000ms
`
	stats, err = ParsePingOutput(loss)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sent != 10 || stats.Recv != 0 || stats.LossPct != 100 || stats.HasRTT {
		t.Fatalf("stats = %+v", stats)
	}

	if _, err := ParsePingOutput("ping: unknown host"); err == nil {
		t.Fatal("expected error")
	}
}
