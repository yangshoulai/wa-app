package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"
)

type waSafeEnvelope struct {
	Body          string
	Enc           string
	Authorization string
}

type nativeSoftwareAttestation struct {
	PrivateKeyPKCS8     string `json:"private_key_pkcs8,omitempty"`
	CertificateChainDER string `json:"certificate_chain_der,omitempty"`
}

var androidKeyAttestationOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}

func buildWASafeEnvelope(plain []byte, serverPublicKeyHex string, attestation nativeSoftwareAttestation) (waSafeEnvelope, error) {
	enc, err := encryptWASafe(plain, serverPublicKeyHex)
	if err != nil {
		return waSafeEnvelope{}, err
	}
	body := "ENC=" + enc
	signature, authorization, err := attestation.sign([]byte(enc))
	if err != nil {
		return waSafeEnvelope{}, err
	}
	return waSafeEnvelope{Body: body + "&H=" + signature, Enc: enc, Authorization: authorization}, nil
}

func ensureNativeSoftwareAttestation(state *nativeState) error {
	if state == nil || state.Attestation.ready() {
		return nil
	}
	challenge, err := nativeAttestationChallenge(*state)
	if err != nil {
		return err
	}
	attestation, err := newNativeSoftwareAttestation(challenge, time.Now().UTC())
	if err != nil {
		return err
	}
	state.Attestation = attestation
	return nil
}

func nativeAttestationChallenge(state nativeState) ([]byte, error) {
	if state.AuthKey != "" {
		return decodeB64Any(state.AuthKey)
	}
	return state.ChatStatic.publicBytes()
}

func newNativeSoftwareAttestation(clientStaticPublic []byte, now time.Time) (nativeSoftwareAttestation, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nativeSoftwareAttestation{}, err
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nativeSoftwareAttestation{}, err
	}
	certificateChainDER, err := newNativeSoftwareAttestationCertificateChain(privateKey, clientStaticPublic, now)
	if err != nil {
		return nativeSoftwareAttestation{}, err
	}
	return nativeSoftwareAttestation{
		PrivateKeyPKCS8:     b64u(privateKeyDER),
		CertificateChainDER: b64u(certificateChainDER),
	}, nil
}

func newNativeSoftwareAttestationCertificateChain(privateKey *ecdsa.PrivateKey, clientStaticPublic []byte, now time.Time) ([]byte, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootSerial, err := nativeAttestationSerial()
	if err != nil {
		return nil, err
	}
	leafSerial, err := nativeAttestationSerial()
	if err != nil {
		return nil, err
	}
	extension, err := nativeSoftwareAndroidKeyAttestationExtension(nativeAndroidKeyAttestationChallenge(clientStaticPublic, now))
	if err != nil {
		return nil, err
	}
	root := &x509.Certificate{
		SerialNumber:          rootSerial,
		Subject:               pkix.Name{CommonName: "Android Keystore Attestation Root"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	leaf := &x509.Certificate{
		SerialNumber: leafSerial,
		Subject:      pkix.Name{CommonName: "Android Keystore Software Attestation"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{
			Id:    androidKeyAttestationOID,
			Value: extension,
		}},
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &privateKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	return append(append([]byte{}, rootDER...), leafDER...), nil
}

func nativeAttestationSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func nativeAndroidKeyAttestationChallenge(clientStaticPublic []byte, now time.Time) []byte {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]byte, 0, len(clientStaticPublic)+9)
	out = binary.BigEndian.AppendUint64(out, uint64(now.Unix()))
	out = append(out, 0x1f)
	out = append(out, clientStaticPublic...)
	return out
}

type nativeSoftwareAndroidKeyDescription struct {
	AttestationVersion       int
	AttestationSecurityLevel asn1.Enumerated
	KeymasterVersion         int
	KeymasterSecurityLevel   asn1.Enumerated
	AttestationChallenge     []byte
	UniqueID                 []byte
	SoftwareEnforced         asn1.RawValue
	TEEEnforced              asn1.RawValue
}

func nativeSoftwareAndroidKeyAttestationExtension(challenge []byte) ([]byte, error) {
	emptyAuthorizationList := asn1.RawValue{FullBytes: []byte{0x30, 0x00}}
	return asn1.Marshal(nativeSoftwareAndroidKeyDescription{
		AttestationVersion:       3,
		AttestationSecurityLevel: 0,
		KeymasterVersion:         4,
		KeymasterSecurityLevel:   0,
		AttestationChallenge:     append([]byte{}, challenge...),
		UniqueID:                 []byte{},
		SoftwareEnforced:         emptyAuthorizationList,
		TEEEnforced:              emptyAuthorizationList,
	})
}

func (a nativeSoftwareAttestation) ready() bool {
	if a.PrivateKeyPKCS8 == "" || a.CertificateChainDER == "" {
		return false
	}
	certificateDER, err := decodeB64Any(a.CertificateChainDER)
	if err != nil {
		return false
	}
	certificates, err := x509.ParseCertificates(certificateDER)
	if err != nil || len(certificates) == 0 {
		return false
	}
	return certificates[0].IsCA
}

func (a nativeSoftwareAttestation) sign(body []byte) (string, string, error) {
	privateKeyDER, err := decodeB64Any(a.PrivateKeyPKCS8)
	if err != nil {
		return "", "", err
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(privateKeyDER)
	if err != nil {
		return "", "", err
	}
	privateKey, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", "", fmt.Errorf("native software attestation key is not ECDSA")
	}
	digest := sha256.Sum256(body)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		return "", "", err
	}
	certificateDER, err := decodeB64Any(a.CertificateChainDER)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(signature), base64.StdEncoding.EncodeToString(certificateDER), nil
}
