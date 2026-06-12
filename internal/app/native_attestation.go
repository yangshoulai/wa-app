package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"time"
)

type waSafeEnvelope struct {
	Body          string
	Enc           string
	Authorization string
}

func buildWASafeEnvelope(plain []byte, serverPublicKeyHex string) (waSafeEnvelope, error) {
	enc, err := encryptWASafe(plain, serverPublicKeyHex)
	if err != nil {
		return waSafeEnvelope{}, err
	}
	signature, authorization, err := signWASafeAttestation(enc)
	if err != nil {
		return waSafeEnvelope{}, err
	}
	return waSafeEnvelope{Body: "ENC=" + enc + "&H=" + signature, Enc: enc, Authorization: authorization}, nil
}

func signWASafeAttestation(enc string) (string, string, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	certificate, err := selfSignedAttestationCertificate(privateKey)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(enc))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(signature), base64.StdEncoding.EncodeToString(certificate), nil
}

func selfSignedAttestationCertificate(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Android Keystore Key"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	return x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
}
