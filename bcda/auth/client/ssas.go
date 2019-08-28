package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var ssasLogger *logrus.Logger

// SSASClient is a client for interacting with the System-to-System Authentication Service.
type SSASClient struct {
	http.Client
	baseURL string
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

func init() {
	ssasLogger = logrus.New()
	ssasLogger.Formatter = &logrus.JSONFormatter{}
	ssasLogger.SetReportCaller(true)
	filePath := os.Getenv("BCDA_SSAS_LOG")

	/* #nosec -- 0640 permissions required for Splunk ingestion */
	file, err := os.OpenFile(filepath.Clean(filePath), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)

	if err == nil {
		ssasLogger.SetOutput(file)
	} else {
		ssasLogger.Info("Failed to open SSAS log file; using default stderr")
	}
}

// NewSSASClient creates and returns an SSASClient.
func NewSSASClient() (*SSASClient, error) {
	var (
		transport = &http.Transport{}
		err       error
	)
	if os.Getenv("SSAS_USE_TLS") == "true" {
		transport, err = tlsTransport()
		if err != nil {
			return nil, errors.Wrap(err, "SSAS client could not be created")
		}
	}

	var timeout int
	if timeout, err = strconv.Atoi(os.Getenv("SSAS_TIMEOUT_MS")); err != nil {
		ssasLogger.Info("Could not get SSAS timeout from environment variable; using default value of 500.")
		timeout = 500
	}

	ssasURL := os.Getenv("SSAS_URL")
	if ssasURL == "" {
		return nil, errors.New("SSAS client could not be created: no URL provided")
	}

	c := http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Millisecond}

	return &SSASClient{c, ssasURL}, nil
}

func tlsTransport() (*http.Transport, error) {
	certFile := os.Getenv("SSAS_CLIENT_CERT_FILE")
	keyFile := os.Getenv("SSAS_CLIENT_KEY_FILE")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "could not load SSAS keypair")
	}

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	caFile := os.Getenv("SSAS_CLIENT_CA_FILE")
	caCert, err := ioutil.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, errors.Wrap(err, "could not read CA file")
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("could not append CA certificate(s)")
	}

	tlsConfig.RootCAs = caCertPool
	tlsConfig.BuildNameToCertificate()

	return &http.Transport{TLSClientConfig: tlsConfig}, nil
}

// CreateGroup POSTs to the SSAS /group endpoint to create a system.
func (c *SSASClient) CreateGroup(id, name string) ([]byte, error) {
	b := fmt.Sprintf(`{"id": "%s", "name": "%s", "scopes": ["bcda-api"]}`, id, name)

	resp, err := c.Post(fmt.Sprintf("%s/group", c.baseURL), "application/json", strings.NewReader(b))
	if err != nil {
		return nil, errors.Wrap(err, "could not create group")
	}

	rb, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "could not create group")
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, errors.Errorf("could not create group: %s", rb)
	}

	return rb, nil
}

// DeleteGroup DELETEs to the SSAS /group/{id} endpoint to delete a group.
func (c *SSASClient) DeleteGroup(id int) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/group/%d", c.baseURL, id), nil)
	if err != nil {
		return errors.Wrap(err, "could not delete group")
	}

	resp, err := c.Do(req)
	if err != nil {
		return errors.Wrap(err, "could not delete group")
	}

	if resp.StatusCode != http.StatusOK {
		rb, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return errors.Wrap(err, "could not delete group")
		}
		return errors.Errorf("could not delete group: %s", rb)
	}

	return nil
}

// CreateSystem POSTs to the SSAS /system endpoint to create a system.
func (c *SSASClient) CreateSystem(clientName, groupID, scope, publicKey, trackingID string) ([]byte, error) {
	type system struct {
		ClientName string `json:"client_name"`
		GroupID    string `json:"group_id"`
		Scope      string `json:"scope"`
		PublicKey  string `json:"public_key"`
		TrackingID string `json:"tracking_id"`
	}

	sys := system{
		ClientName: clientName,
		GroupID:    groupID,
		Scope:      scope,
		PublicKey:  publicKey,
		TrackingID: trackingID,
	}

	bb, err := json.Marshal(sys)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create system")
	}
	br := bytes.NewReader(bb)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/system", c.baseURL), br)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create system")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create system")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, errors.New(fmt.Sprintf("failed to create system. status code: %v", resp.StatusCode))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	return body, nil
}

// GetPublicKey GETs the SSAS /system/{systemID}/key endpoint to retrieve a system's public key.
func (c *SSASClient) GetPublicKey(systemID int) ([]byte, error) {
	resp, err := c.Get(fmt.Sprintf("%s/system/%v/key", c.baseURL, systemID))
	if err != nil {
		return nil, errors.Wrap(err, "could not get public key")
	}

	defer resp.Body.Close()

	var respMap map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&respMap); err != nil {
		return nil, errors.Wrap(err, "could not get public key")
	}

	return []byte(respMap["public_key"]), nil
}

// ResetCredentials PUTs to the SSAS /system/{systemID}/credentials endpoint to reset the system's credentials.
func (c *SSASClient) ResetCredentials(systemID string) ([]byte, error) {
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/system/%s/credentials", c.baseURL, systemID), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reset credentials")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reset credentials")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, errors.New(fmt.Sprintf("failed to reset credentials. status code: %v", resp.StatusCode))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reset credentials")
	}

	return body, nil

}

// DeleteCredentials DELETEs from the SSAS /system/{systemID}/credentials endpoint to deactivate credentials associated with the system.
func (c *SSASClient) DeleteCredentials(systemID string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/system/%s/credentials", c.baseURL, systemID), nil)
	if err != nil {
		return errors.Wrap(err, "failed to delete credentials")
	}

	resp, err := c.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to delete credentials")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Wrap(err, "failed to delete credentials")
	}

	return nil
}

// RevokeAccessToken DELETEs to the public SSAS /token endpoint to revoke the token
func (c *SSASClient) RevokeAccessToken(tokenID string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/token/%s", c.baseURL, tokenID), nil)
	if err != nil {
		return errors.Wrap(err, "bad request structure")
	}

	resp, err := c.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to revoke token")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to revoke token; %v", resp.StatusCode)
	}

	return nil
}

// GetToken POSTs to the public SSAS /token endpoint to get an access token for a BCDA client
func (c *SSASClient) GetToken(credentials Credentials) ([]byte, error) {
	public := os.Getenv("SSAS_PUBLIC_URL")
	url := fmt.Sprintf("%s/token", public)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "bad request structure")
	}

	req.SetBasicAuth(credentials.ClientID, credentials.ClientSecret)
	req.Header.Add("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "token request failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed; %v", resp.StatusCode)
	}

	var t = TokenResponse{}
	if err = json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, errors.Wrap(err, "could not decode token response")
	}

	return []byte(t.AccessToken), nil
}

// VerifyPublicToken verifies that the tokenString presented was issued by the public server. It does so using
// the introspect endpoint as defined by https://tools.ietf.org/html/rfc7662
func (c *SSASClient) VerifyPublicToken(tokenString string) ([]byte, error) {
	public := os.Getenv("SSAS_PUBLIC_URL")
	url := fmt.Sprintf("%s/introspect", public)
	body, err := json.Marshal(struct {
		Token string `json:"token"`
	}{Token: tokenString})
	if err != nil {
		return nil, errors.Wrap(err, "bad request structure")
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "bad request structure")
	}

	// TODO assuming auth is by self-management
	clientID := os.Getenv("BCDA_SSAS_CLIENT_ID")
	secret := os.Getenv("BCDA_SSAS_SECRET")
	if clientID == "" || secret == "" {
		return nil, errors.New("missing clientID or secret")
	}
	req.SetBasicAuth(clientID, secret)
	req.Header.Add("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "introspect request failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspect request failed; %v", resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "could not read introspect response")
	}

	return b, nil
}
