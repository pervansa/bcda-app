package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"log"
	"os"

	"github.com/CMSgov/bcda-app/bcda/auth"
	"github.com/stretchr/testify/suite"
)

type AuthTestSuite struct {
	suite.Suite
	authBackend *auth.JWTAuthenticationBackend
	tmpFiles    []string
}

func (s *AuthTestSuite) CreateTempFile() (*os.File, error) {
	tmpfile, err := ioutil.TempFile("", "bcda_backend_test_")
	if err != nil {
		return &os.File{}, err
	}

	return tmpfile, nil
}

func (s *AuthTestSuite) SavePrivateKey(f *os.File, key *rsa.PrivateKey) {
	var privateKey = &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	err := pem.Encode(f, privateKey)
	if err != nil {
		log.Fatal(err)
	}
}

func (s *AuthTestSuite) SavePubKey(f *os.File, pubkey rsa.PublicKey) {
	asn1Bytes, err := x509.MarshalPKIXPublicKey(&pubkey)
	if err != nil {
		log.Fatal(err)
	}

	var pemkey = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: asn1Bytes,
	}

	err = pem.Encode(f, pemkey)
	if err != nil {
		log.Fatal(err)
	}
}

func (s *AuthTestSuite) SetupAuthBackend() {
	reader := rand.Reader
	bitSize := 1024

	key, err := rsa.GenerateKey(reader, bitSize)
	if err != nil {
		log.Fatal(err)
	}
	publicKey := key.PublicKey

	privKeyFile, err := s.CreateTempFile()
	if err != nil {
		log.Fatal(err)
	}

	os.Setenv("JWT_PRIVATE_KEY_FILE", privKeyFile.Name())
	s.tmpFiles = append(s.tmpFiles, privKeyFile.Name())
	s.SavePrivateKey(privKeyFile, key)
	defer privKeyFile.Close()

	pubKeyFile, err := s.CreateTempFile()
	if err != nil {
		log.Fatal(err)
	}

	os.Setenv("JWT_PUBLIC_KEY_FILE", pubKeyFile.Name())
	s.tmpFiles = append(s.tmpFiles, pubKeyFile.Name())
	s.SavePubKey(pubKeyFile, publicKey)
	defer pubKeyFile.Close()

	s.authBackend = auth.InitAuthBackend()
}

func (s *AuthTestSuite) TearDownTest() {
	for _, f := range s.tmpFiles {
		os.Remove(f)
	}
}
