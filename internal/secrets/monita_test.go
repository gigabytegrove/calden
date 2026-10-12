package secrets

import (
    "strings"
    "testing"
)

func TestMonitaTokenEncryptionRoundTrip(t *testing.T) {
    key := []byte(strings.Repeat("s", 48))
    token := "SA.test-random-monita-token"
    sealed, err := Seal(key, token)
    if err != nil { t.Fatal(err) }
    if !strings.HasPrefix(sealed, prefix) || strings.Contains(sealed, token) {
        t.Fatal("token was not encrypted")
    }
    plain, err := Open(key, sealed)
    if err != nil || plain != token { t.Fatalf("round trip: %q %v", plain, err) }
    // No legacy credential disruption during migration.
    legacy, err := Open(key, token)
    if err != nil || legacy != token { t.Fatalf("legacy token: %q %v", legacy, err) }
}

func TestMonitaTokenWrongKeyAndTampering(t *testing.T) {
    key := []byte(strings.Repeat("s", 48))
    sealed, err := Seal(key, "SA.secret")
    if err != nil { t.Fatal(err) }
    _, err = Open([]byte(strings.Repeat("x", 48)), sealed)
    if err == nil { t.Fatal("wrong key should reject encrypted data") }
    _, err = Open(key, sealed+"tampered")
    if err == nil { t.Fatal("tampering must be detected") }
}
