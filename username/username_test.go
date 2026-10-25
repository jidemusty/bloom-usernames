package username

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"BOB", "bob", false},
		{"   charlie   ", "charlie", false},
		{"sam", "sam", false},
		{"abcdefghijklmnopqrst", "abcdefghijklmnopqrst", false},
		{"jo", "", true},
		{"abcdefghijklmnopqrstu", "", true},
		{"john doe", "", true},
		{"bob-1", "", true},
		{"bob😀", "", true},
		{"", "", true},
	}

	for _, c := range cases {
		got, err := Normalize(c.input)

		if (err != nil) != c.wantErr {
			t.Errorf("Normalize(%q) unexpected error state: got err=%v, wantErr=%t", c.input, err, c.wantErr)
			continue
		}

		if got != c.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", c.input, got, err, c.want)
		}
	}
}
