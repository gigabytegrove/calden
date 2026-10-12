package secrets

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "io"
    "strings"
)

const prefix = "enc:v1:"

func derive(master []byte) [32]byte {
    return sha256.Sum256(append(append([]byte(nil), master...), []byte("/calden/monita-token/v1")...))
}

// Seal encrypts a Monita token with Calden's persistent signing secret using
// authenticated AES-GCM. No application credentials enter source or logs.
func Seal(master []byte, plaintext string) (string, error) {
    if plaintext == "" { return "", nil }
    if len(master) < 32 { return "", errors.New("invalid encryption key") }
    key := derive(master)
    block, err := aes.NewCipher(key[:]); if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block); if err != nil { return "", err }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
    out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return prefix + base64.RawURLEncoding.EncodeToString(out), nil
}

// Open accepts legacy plaintext tokens for backwards compatibility. The next
// settings save migrates them to authenticated encryption automatically.
func Open(master []byte, stored string) (string, error) {
    if !strings.HasPrefix(stored, prefix) { return stored, nil }
    if len(master) < 32 { return "", errors.New("invalid encryption key") }
    raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
    if err != nil { return "", err }
    key := derive(master)
    block, err := aes.NewCipher(key[:]); if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block); if err != nil { return "", err }
    if len(raw) < gcm.NonceSize() { return "", errors.New("invalid encrypted token") }
    plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
    if err != nil { return "", err }
    return string(plain), nil
}
