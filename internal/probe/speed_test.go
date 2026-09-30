package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClassifyLimit(t *testing.T) {
	cases := []struct {
		name    string
		read    int64
		max     int64
		elapsed time.Duration
		maxDur  time.Duration
		hitTime bool
		hitEOF  bool
		want    string
	}{
		{name: "bytes", read: 32, max: 32, elapsed: time.Second, maxDur: 8 * time.Second, want: LimitedByBytes},
		{name: "time flag", read: 10, max: 32, elapsed: time.Second, maxDur: 8 * time.Second, hitTime: true, want: LimitedByTime},
		{name: "time elapsed", read: 10, max: 32, elapsed: 8 * time.Second, maxDur: 8 * time.Second, want: LimitedByTime},
		{name: "eof", read: 5, max: 32, elapsed: time.Second, maxDur: 8 * time.Second, hitEOF: true, want: LimitedByEOF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyLimit(tc.read, tc.max, tc.elapsed, tc.maxDur, tc.hitTime, tc.hitEOF)
			if got != tc.want {
				t.Fatalf("got %s", got)
			}
		})
	}
}

func TestMbps(t *testing.T) {
	if got := Mbps(1_000_000, time.Second); got != 8 {
		t.Fatalf("mbps = %v", got)
	}
	if got := Mbps(0, time.Second); got != 0 {
		t.Fatalf("empty = %v", got)
	}
}

func TestDownloadLimits(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(make([]byte, 64*1024))
		}))
		defer srv.Close()

		res, err := Download(t.Context(), srv.URL, 5*time.Second, 4096)
		if err != nil {
			t.Fatal(err)
		}
		if res.LimitedBy != LimitedByBytes || res.Bytes != 4096 {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("eof", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hello"))
		}))
		defer srv.Close()

		res, err := Download(t.Context(), srv.URL, 5*time.Second, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if res.LimitedBy != LimitedByEOF || res.Bytes != 5 {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("time", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
		}))
		defer srv.Close()

		res, err := Download(t.Context(), srv.URL, 200*time.Millisecond, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if res.LimitedBy != LimitedByTime || res.Bytes < 5 {
			t.Fatalf("result = %+v", res)
		}
	})
}
