package ipnotes

import "testing"

func TestNormalizeAndValidate(t *testing.T) {
	for raw, want := range map[string]string{
		" 121.43.212.142 ": "121.43.212.142", "[2001:0db8:0:0::1]": "2001:db8::1", "::ffff:121.43.212.142": "121.43.212.142", "fe80::1%en0": "fe80::1",
		"121.43.212.142:443": "", "example.com": "", "*": "", "192.168.1.0/24": "",
	} {
		if got := NormalizeIP(raw); got != want {
			t.Fatalf("%q => %q want %q", raw, got, want)
		}
	}
	n, err := Validate(Note{IP: "::ffff:121.43.212.142", Name: " 伊智生产环境官网 ", Group: " 生产 "})
	if err != nil || n.IP != "121.43.212.142" || n.Name != "伊智生产环境官网" || n.Group != "生产" {
		t.Fatal(n, err)
	}
	if _, err := Validate(Note{IP: "1.1.1.1"}); err == nil {
		t.Fatal("empty label accepted")
	}
}
