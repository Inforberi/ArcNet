package releaseversion

import "testing"

func TestOrdering(t *testing.T) {
	chain := []string{"0.7.0-stage5-a7", "0.7.0-stage5-a9", "0.7.0-stage5-a10", "0.7.0-rc.1", "0.7.0", "0.7.1-alpha.2", "0.7.1-alpha.10", "0.7.1-beta", "0.7.1", "0.8.0", "1.0.0", "10.0.0"}
	for i, a := range chain {
		for j, b := range chain {
			n, ok := Compare(a, b)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if !ok || n != want {
				t.Fatalf("ordering %s / %s: %d", a, b, n)
			}
		}
	}
	for _, bad := range []string{"unknown", "1.2", "v1.2.3", "01.2.3", "1.2.3-01", "1.2.3-", "1.2.3+", "1.2.3-alpha..x", "1.2.3/x"} {
		if _, ok := Parse(bad); ok {
			t.Fatal("accepted", bad)
		}
	}
	if n, ok := Compare("1.2.3+one", "1.2.3+two"); !ok || n != 0 {
		t.Fatal("build metadata affects precedence")
	}
	if Publishable("1.2.3+build") {
		t.Fatal("ambiguous public release accepted")
	}
	if n, ok := Compare("0.7.0-stage5-a10+receipt", "0.7.0-stage5-a10"); !ok || n != 0 {
		t.Fatal("legacy build metadata affects precedence")
	}
}
