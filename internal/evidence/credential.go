package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"wildframe/internal/domain"
)

type CredentialEnvelope struct {
	domain.ReleaseCredential
	PublicKey string `json:"publicKey"`
}

type Signer struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func NewSigner(private []byte) (*Signer, error) {
	if len(private) == 0 {
		public, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		return &Signer{private: key, public: public}, nil
	}
	if len(private) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("Ed25519 私钥长度无效")
	}
	key := ed25519.PrivateKey(append([]byte(nil), private...))
	public := key.Public().(ed25519.PublicKey)
	return &Signer{private: key, public: public}, nil
}

func (s *Signer) PrivateKey() []byte { return append([]byte(nil), s.private...) }

func (s *Signer) Issue(id, collectionID, digest, issuer string, version int, now time.Time) CredentialEnvelope {
	credential := domain.ReleaseCredential{
		CredentialID: id, CollectionID: collectionID, ManifestDigest: digest,
		ManifestVersion: version, IssuedBy: issuer, IssuedAt: now.UTC(), Algorithm: "Ed25519",
		VerificationStatus: "valid",
	}
	credential.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.private, credentialPayload(credential)))
	return CredentialEnvelope{ReleaseCredential: credential, PublicKey: base64.RawURLEncoding.EncodeToString(s.public)}
}

func VerifyCredential(envelope CredentialEnvelope, expectedDigest string) (bool, string) {
	if ok, message := validateCredentialStructure(envelope, expectedDigest == ""); !ok {
		return false, message
	}
	if envelope.Algorithm != "Ed25519" {
		return false, "不支持的签名算法"
	}
	if expectedDigest != "" && envelope.ManifestDigest != expectedDigest {
		return false, "凭据清单摘要与冻结清单不一致"
	}
	public, err := base64.RawURLEncoding.DecodeString(envelope.PublicKey)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return false, "公钥格式无效"
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false, "签名格式无效"
	}
	if !ed25519.Verify(ed25519.PublicKey(public), credentialPayload(envelope.ReleaseCredential), signature) {
		return false, "签名验证失败"
	}
	return true, "凭据有效，清单未被篡改"
}

var credentialDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidateCredentialStructure(envelope CredentialEnvelope) (bool, string) {
	return validateCredentialStructure(envelope, true)
}

func validateCredentialStructure(envelope CredentialEnvelope, strictDigest bool) (bool, string) {
	if strings.TrimSpace(envelope.CredentialID) == "" || strings.TrimSpace(envelope.CollectionID) == "" || strings.TrimSpace(envelope.IssuedBy) == "" || envelope.IssuedAt.IsZero() || envelope.ManifestVersion <= 0 || (strictDigest && !credentialDigestPattern.MatchString(envelope.ManifestDigest)) || (!strictDigest && strings.TrimSpace(envelope.ManifestDigest) == "") {
		return false, "凭据必需字段或时间格式无效"
	}
	if envelope.Algorithm != "Ed25519" {
		return false, "不支持的签名算法"
	}
	public, err := base64.RawURLEncoding.DecodeString(envelope.PublicKey)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return false, "公钥格式无效"
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false, "签名格式无效"
	}
	return true, "凭据结构有效"
}

func credentialPayload(c domain.ReleaseCredential) []byte {
	return []byte(fmt.Sprintf("wildframe-credential-v1\n%s\n%s\n%s\n%d\n%s\n%s\n%s", c.CredentialID, c.CollectionID, c.ManifestDigest, c.ManifestVersion, c.IssuedBy, c.IssuedAt.UTC().Format(time.RFC3339Nano), c.Algorithm))
}
