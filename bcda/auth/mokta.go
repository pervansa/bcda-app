package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/dgrijalva/jwt-go"
)

type Mokta struct {
	privateKey  *rsa.PrivateKey
	publicKey   rsa.PublicKey
	publicKeyID string
	serverID    string
}

func NewMokta() *Mokta {
	reader := rand.Reader
	bitSize := 1024

	privateKey, err := rsa.GenerateKey(reader, bitSize)
	if err != nil {
		log.Fatal(err)
	}
	publicKey := privateKey.PublicKey

	keys := make(map[string]rsa.PublicKey)
	keys["mokta"] = publicKey

	return &Mokta{privateKey, publicKey, "mokta", "mokta.fake.backend",}
}

func (m *Mokta) PublicKeyFor(id string) (rsa.PublicKey, bool) {
	if id != m.publicKeyID {
		return rsa.PublicKey{}, false
	}
	return m.publicKey, true
}

func (m *Mokta) ServerID() string {
	return m.serverID
}

func (m *Mokta) AddClientApplication(localId string) (string, string, error) {
	id, err := someRandomBytes(16)
	if err != nil {
		return "", "", nil
	}
	key, err := someRandomBytes(32)
	if err != nil {
		return "", "", nil
	}

	return base64.URLEncoding.EncodeToString(id), base64.URLEncoding.EncodeToString(key), err
}

func randomClientID() string {
	b, err := someRandomBytes(4)
	if err != nil {
		return "not_a_random_client_id"
	}
	return fmt.Sprintf("%x", b)
}

func someRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Returns a new access token
func (m *Mokta) NewToken(clientID string) (string, error) {
	return m.NewCustomToken(OktaToken{
		m.publicKeyID,
		m.serverID,
		500,
		clientID,
		[]string{"bcda_api"},
		clientID,
	})
}

type OktaToken struct {
	KeyID     string   `json:"kid,omitempty"`
	Issuer    string   `json:"iss,omitempty"`
	ExpiresIn int64    `json:"exp,omitempty"`
	ClientID  string   `json:"cid,omitempty"`
	Scopes    []string `json:"scp,omitempty"`
	Subject   string   `json:"sub,omitempty"`
}

func (m *Mokta) NewCustomToken(overrides OktaToken) (string, error) {
	values := m.valuesWithOverrides(overrides)
	tid, err := someRandomBytes(32)
	if err != nil {
		return "", err
	}

	token := jwt.New(jwt.SigningMethodRS256)
	token.Header["kid"] = values.KeyID
	token.Claims = jwt.MapClaims{
		"ver": 1,
		"jti": base64.URLEncoding.EncodeToString(tid),
		"iss": values.Issuer,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * time.Duration(values.ExpiresIn)).Unix(),
		"cid": values.ClientID,
		"scp": values.Scopes,
		"sub": values.Subject,
	}

	tokenString, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (m *Mokta) valuesWithOverrides(or OktaToken) OktaToken {
	cid := randomClientID()
	v := OktaToken{
		m.publicKeyID,
		m.serverID,
		500,
		cid,
		[]string{"bcda_api"},
		cid,
	}

	if or.ClientID != "" {
		v.ClientID = or.ClientID
	}

	if or.ExpiresIn != 0 {
		v.ExpiresIn = or.ExpiresIn
	}

	if or.Issuer != "" {
		v.Issuer = or.Issuer
	}

	if or.KeyID != "" {
		v.KeyID = or.KeyID
	}

	if len(or.Scopes) != 0 {
		v.Scopes = make([]string, len(or.Scopes))
		copy(v.Scopes, or.Scopes)
	}

	if or.Subject != "" {
		v.Subject = or.Subject
	}

	return v
}
