package appidentity

import "testing"

func TestBundleAndSignatureDeclarations(t *testing.T) {
	var info Info
	applyBundle([]byte(`{"CFBundleDisplayName":"Example App","CFBundleName":"Other","CFBundleIdentifier":"com.example.app","CFBundleShortVersionString":"1.2.3"}`), &info)
	applySignature("Authority=Developer ID Application: Example Inc (123)\nAuthority=Root CA\nTeamIdentifier=123\n", &info)
	if info.AppName != "Example App" || info.BundleID != "com.example.app" || info.Version != "1.2.3" || info.Signer != "Developer ID Application: Example Inc (123)" || info.TeamID != "123" || info.Signature != "" {
		t.Fatalf("declarations must not imply verification: %+v", info)
	}
	info = Info{}
	applySignature("Signature=adhoc\nTeamIdentifier=not set\n", &info)
	if info.Signer != "" || info.TeamID != "" {
		t.Fatal("ad hoc signature must not claim a publisher")
	}
	applyBundle([]byte(`{"CFBundleName":"Fallback","CFBundleVersion":"42"}`), &info)
	if info.AppName != "Fallback" || info.Version != "42" {
		t.Fatal(info)
	}
}
