package social

import "testing"

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`a%b_c\d`)
	want := `a\%b\_c\\d`
	if got != want {
		t.Fatalf("escapeLike: got %q want %q", got, want)
	}
}

func TestMaskEmail(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"burak.kocaolu@gmail.com", "b***@gmail.com"},
		{"a@x.com", "a***@x.com"},
		{"ab@x.com", "a***@x.com"},
		{"", "***"},
		{"nodomain", "***"},
		{"@x.com", "***"},
	}
	for _, c := range cases {
		if got := maskEmail(c.in); got != c.want {
			t.Errorf("maskEmail(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestLooksLikeEmail(t *testing.T) {
	if !looksLikeEmail("burak@gmail.com") {
		t.Fatal("expected email")
	}
	if looksLikeEmail("Burak") {
		t.Fatal("name should not look like email")
	}
}
