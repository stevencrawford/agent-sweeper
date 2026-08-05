package units

import "testing"

func TestBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KiB"},
		{1536, "1.5KiB"},
		{1 << 20, "1.0MiB"},
		{300 << 10, "300.0KiB"},
	}
	for _, c := range cases {
		if got := Bytes(c.n); got != c.want {
			t.Fatalf("Bytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
