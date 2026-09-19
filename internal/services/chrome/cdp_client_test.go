package chrome

import "testing"

func TestFilterGoogleCookiesUsesExactDomainsAndStableOrder(t *testing.T) {
	header, hasPSID, hasPSIDTS := FilterGoogleCookies([]CDPCookie{
		{Name: "SID", Value: "sid", Domain: ".accounts.google.com"},
		{Name: "__Secure-1PSID", Value: "psid", Domain: ".flow.google.com"},
		{Name: "__Secure-1PSIDTS", Value: "ts", Domain: ".gemini.google.com"},
		{Name: "SHOULD_NOT_PASS", Value: "secret", Domain: "google.com.attacker.example"},
	})

	if !hasPSID || !hasPSIDTS {
		t.Fatalf("expected Google session cookie flags, got psid=%v psidts=%v", hasPSID, hasPSIDTS)
	}
	if header != "SID=sid; __Secure-1PSID=psid; __Secure-1PSIDTS=ts" {
		t.Fatalf("unexpected stable cookie header: %q", header)
	}
	if contains := "SHOULD_NOT_PASS"; len(header) > 0 && hasSubstring(header, contains) {
		t.Fatalf("cookie from attacker domain was included")
	}
}

func hasSubstring(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
