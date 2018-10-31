package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/CMSgov/bcda-app/bcda/auth"
	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/models"
	"github.com/CMSgov/bcda-app/bcda/responseutils"
	"github.com/CMSgov/bcda-app/bcda/testUtils"
	que "github.com/bgentry/que-go"
	jwt "github.com/dgrijalva/jwt-go"
	fhirmodels "github.com/eug48/fhir/models"
	"github.com/go-chi/chi"
	"github.com/jackc/pgx"
	"github.com/jinzhu/gorm"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type APITestSuite struct {
	testUtils.AuthTestSuite
	rr *httptest.ResponseRecorder
	db *gorm.DB
}

func (s *APITestSuite) SetupTest() {
	models.InitializeGormModels()
	s.db = database.GetGORMDbConnection()
	s.rr = httptest.NewRecorder()
}

func (s *APITestSuite) TestBulkRequest() {
	s.SetupAuthBackend()

	acoID := "0c527d2e-2e8a-4808-b11d-0fa06baf8254"
	user, err := s.AuthBackend.CreateUser("api.go Test User", "testbulkrequest@example.com", uuid.Parse(acoID))
	if err != nil {
		s.T().Error(err)
	}

	token := jwt.New(jwt.SigningMethodRS512)
	token.Claims = jwt.MapClaims{
		"sub": user.UUID.String(),
		"aco": acoID,
		"id":  uuid.NewRandom().String(),
	}
	token.Valid = true

	req := httptest.NewRequest("GET", "/api/v1/ExplanationOfBenefit/$export", nil)
	req = req.WithContext(context.WithValue(req.Context(), "token", token))

	queueDatabaseURL := os.Getenv("QUEUE_DATABASE_URL")
	pgxcfg, err := pgx.ParseURI(queueDatabaseURL)
	if err != nil {
		s.T().Error(err)
	}

	pgxpool, err := pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig:   pgxcfg,
		AfterConnect: que.PrepareStatements,
	})
	if err != nil {
		s.T().Error(err)
	}
	defer pgxpool.Close()

	qc = que.NewClient(pgxpool)

	handler := http.HandlerFunc(bulkRequest)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusAccepted, s.rr.Code)
	s.db.Where("user_id = ?", user.UUID).Delete(models.Job{})
	s.db.Where("uuid = ?", user.UUID).Delete(auth.User{})
}

func (s *APITestSuite) TestBulkRequestNoBeneficiariesInACO() {
	s.SetupAuthBackend()

	userID := "82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"
	acoID := "DBBD1CE1-AE24-435C-807D-ED45953077D3"

	token := jwt.New(jwt.SigningMethodRS512)
	token.Claims = jwt.MapClaims{
		"sub": userID,
		"aco": acoID,
		"id":  uuid.NewRandom().String(),
	}
	token.Valid = true

	req := httptest.NewRequest("GET", "/api/v1/ExplanationOfBenefit/$export", nil)
	req = req.WithContext(context.WithValue(req.Context(), "token", token))

	queueDatabaseURL := os.Getenv("QUEUE_DATABASE_URL")
	pgxcfg, err := pgx.ParseURI(queueDatabaseURL)
	if err != nil {
		s.T().Error(err)
	}

	pgxpool, err := pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig:   pgxcfg,
		AfterConnect: que.PrepareStatements,
	})
	if err != nil {
		s.T().Error(err)
	}
	defer pgxpool.Close()

	qc = que.NewClient(pgxpool)

	handler := http.HandlerFunc(bulkRequest)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusAccepted, s.rr.Code)
}

func (s *APITestSuite) TestBulkRequestMissingToken() {
	req := httptest.NewRequest("GET", "/api/v1/ExplanationOfBenefit/$export", nil)

	handler := http.HandlerFunc(bulkRequest)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusUnauthorized, s.rr.Code)

	var respOO fhirmodels.OperationOutcome
	err := json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.TokenErr, respOO.Issue[0].Details.Coding[0].Display)
}

func (s *APITestSuite) TestBulkRequestUserDoesNotExist() {
	s.SetupAuthBackend()

	acoID := "dbbd1ce1-ae24-435c-807d-ed45953077d3"
	subID := "82503a18-bf3b-436d-ba7b-bae09b7ffdff"
	tokenID := "665341c9-7d0c-4844-b66f-5910d9d0822f"

	token := jwt.New(jwt.SigningMethodRS512)
	token.Claims = jwt.MapClaims{
		"sub": subID,
		"aco": acoID,
		"id":  tokenID,
	}
	token.Valid = true

	req := httptest.NewRequest("GET", "/api/v1/ExplanationOfBenefit/$export", nil)
	req = req.WithContext(context.WithValue(req.Context(), "token", token))

	handler := http.HandlerFunc(bulkRequest)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusInternalServerError, s.rr.Code)

	var respOO fhirmodels.OperationOutcome
	err := json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.DbErr, respOO.Issue[0].Details.Coding[0].Display)
}

func (s *APITestSuite) TestBulkRequestNoQueue() {
	qc = nil
	s.SetupAuthBackend()

	acoID := "0c527d2e-2e8a-4808-b11d-0fa06baf8254"
	user, err := s.AuthBackend.CreateUser("api.go Test User", "testbulkrequestnoqueue@example.com", uuid.Parse(acoID))
	if err != nil {
		s.T().Error(err)
	}

	token := jwt.New(jwt.SigningMethodRS512)
	token.Claims = jwt.MapClaims{
		"sub": user.UUID.String(),
		"aco": acoID,
		"id":  uuid.NewRandom().String(),
	}
	token.Valid = true

	req := httptest.NewRequest("GET", "/api/v1/ExplanationOfBenefit/$export", nil)
	req = req.WithContext(context.WithValue(req.Context(), "token", token))
	handler := http.HandlerFunc(bulkRequest)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusInternalServerError, s.rr.Code)

	var respOO fhirmodels.OperationOutcome
	err = json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.Processing, respOO.Issue[0].Details.Coding[0].Display)

	s.db.Where("uuid = ?", user.UUID).Delete(auth.User{})
}

func (s *APITestSuite) TestJobStatusInvalidJobID() {
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%s", "test"), nil)

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", "test")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	var respOO fhirmodels.OperationOutcome
	err := json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.Processing, respOO.Issue[0].Details.Coding[0].Display)
}

func (s *APITestSuite) TestJobStatusJobDoesNotExist() {
	jobID := "1234"
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%s", jobID), nil)

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", jobID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	var respOO fhirmodels.OperationOutcome
	err := json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.DbErr, respOO.Issue[0].Details.Coding[0].Display)
}

func (s *APITestSuite) TestJobStatusPending() {
	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/ExplanationOfBenefit/$export",
		Status:     "Pending",
	}
	s.db.Save(&j)

	req, err := http.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%d", j.ID), nil)
	assert.Nil(s.T(), err)

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", fmt.Sprint(j.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusAccepted, s.rr.Code)
	assert.Equal(s.T(), "Pending", s.rr.Header().Get("X-Progress"))

	s.db.Delete(&j)
}

func (s *APITestSuite) TestJobStatusInProgress() {
	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/ExplanationOfBenefit/$export",
		Status:     "In Progress",
	}
	s.db.Save(&j)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%d", j.ID), nil)

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", fmt.Sprint(j.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusAccepted, s.rr.Code)
	assert.Equal(s.T(), "In Progress", s.rr.Header().Get("X-Progress"))

	s.db.Delete(&j)
}

func (s *APITestSuite) TestJobStatusFailed() {
	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/ExplanationOfBenefit/$export",
		Status:     "Failed",
	}

	s.db.Save(&j)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%d", j.ID), nil)

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", fmt.Sprint(j.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusInternalServerError, s.rr.Code)

	s.db.Delete(&j)
}

func (s *APITestSuite) TestJobStatusCompleted() {
	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/ExplanationOfBenefit/$export",
		Status:     "Completed",
	}
	s.db.Save(&j)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%d", j.ID), nil)
	req.TLS = &tls.ConnectionState{}

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", fmt.Sprint(j.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)
	assert.Equal(s.T(), "application/json", s.rr.Header().Get("Content-Type"))

	var rb bulkResponseBody
	err := json.Unmarshal(s.rr.Body.Bytes(), &rb)
	if err != nil {
		s.T().Error(err)
	}

	expectedurl := fmt.Sprintf("%s/%s/%s", "https://example.com/data", fmt.Sprint(j.ID), "dbbd1ce1-ae24-435c-807d-ed45953077d3.ndjson")

	assert.Equal(s.T(), j.RequestURL, rb.RequestURL)
	assert.Equal(s.T(), true, rb.RequiresAccessToken)
	assert.Equal(s.T(), "ExplanationOfBenefit", rb.Files[0].Type)
	assert.Equal(s.T(), expectedurl, rb.Files[0].URL)
	assert.Empty(s.T(), rb.Errors)

	s.db.Delete(&j)
}

func (s *APITestSuite) TestJobStatusCompletedErrorFileExists() {
	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/ExplanationOfBenefit/$export",
		Status:     "Completed",
	}
	s.db.Save(&j)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%d", j.ID), nil)
	req.TLS = &tls.ConnectionState{}

	handler := http.HandlerFunc(jobStatus)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobId", fmt.Sprint(j.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	f := fmt.Sprintf("%s/%s", os.Getenv("FHIR_PAYLOAD_DIR"), fmt.Sprint(j.ID))
	if _, err := os.Stat(f); os.IsNotExist(err) {
		err = os.MkdirAll(f, os.ModePerm)
		if err != nil {
			s.T().Error(err)
		}
	}

	errFilePath := fmt.Sprintf("%s/%s/%s-error.ndjson", os.Getenv("FHIR_PAYLOAD_DIR"), fmt.Sprint(j.ID), j.AcoID)
	_, err := os.Create(errFilePath)
	if err != nil {
		s.T().Error(err)
	}

	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)
	assert.Equal(s.T(), "application/json", s.rr.Header().Get("Content-Type"))

	var rb bulkResponseBody
	err = json.Unmarshal(s.rr.Body.Bytes(), &rb)
	if err != nil {
		s.T().Error(err)
	}

	dataurl := fmt.Sprintf("%s/%s/%s", "https://example.com/data", fmt.Sprint(j.ID), "dbbd1ce1-ae24-435c-807d-ed45953077d3.ndjson")
	errorurl := fmt.Sprintf("%s/%s/%s", "https://example.com/data", fmt.Sprint(j.ID), "dbbd1ce1-ae24-435c-807d-ed45953077d3-error.ndjson")

	assert.Equal(s.T(), j.RequestURL, rb.RequestURL)
	assert.Equal(s.T(), true, rb.RequiresAccessToken)
	assert.Equal(s.T(), "ExplanationOfBenefit", rb.Files[0].Type)
	assert.Equal(s.T(), dataurl, rb.Files[0].URL)
	assert.Equal(s.T(), "OperationOutcome", rb.Errors[0].Type)
	assert.Equal(s.T(), errorurl, rb.Errors[0].URL)

	s.db.Delete(&j)
	os.Remove(errFilePath)
}

func (s *APITestSuite) TestServeData() {
	os.Setenv("FHIR_PAYLOAD_DIR", "../bcdaworker/data/test")
	req := httptest.NewRequest("GET", "/data/test.ndjson", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("acoID", "test")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler := http.HandlerFunc(serveData)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)
	assert.Contains(s.T(), s.rr.Body.String(), `{"resourceType": "Bundle", "total": 33, "entry": [{"resource": {"status": "active", "diagnosis": [{"diagnosisCodeableConcept": {"coding": [{"system": "http://hl7.org/fhir/sid/icd-9-cm", "code": "2113"}]},`)
}

func (s *APITestSuite) TestGetToken() {
	s.SetupAuthBackend()
	req := httptest.NewRequest("GET", "/api/v1/token", nil)

	handler := http.HandlerFunc(getToken)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)
	assert.NotEmpty(s.T(), s.rr.Body)
}

// TODO: Mock with BB_SERVER_LOCATION
// func (s *APITestSuite) TestBlueButtonMetadata() {
// 	req := httptest.NewRequest("GET", "/api/v1/bb_metadata", nil)

// 	handler := http.HandlerFunc(blueButtonMetadata)
// 	handler.ServeHTTP(s.rr, req)

// 	assert.Equal(s.T(), http.StatusOK, s.rr.Code)

// 	var respCS fhirmodels.CapabilityStatement
// 	err := json.Unmarshal(s.rr.Body.Bytes(), &respCS)
// 	if err != nil {
// 		s.T().Error(err)
// 	}
// }

func (s *APITestSuite) TestBlueButtonMetadataClientError() {
	origBBCertPath := os.Getenv("BB_CLIENT_CERT_FILE")
	os.Setenv("BB_CLIENT_CERT_FILE", "")

	req := httptest.NewRequest("GET", "/api/v1/bb_metadata", nil)

	handler := http.HandlerFunc(blueButtonMetadata)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusInternalServerError, s.rr.Code)

	var respOO fhirmodels.OperationOutcome
	err := json.Unmarshal(s.rr.Body.Bytes(), &respOO)
	if err != nil {
		s.T().Error(err)
	}

	assert.Equal(s.T(), responseutils.Error, respOO.Issue[0].Severity)
	assert.Equal(s.T(), responseutils.Exception, respOO.Issue[0].Code)
	assert.Equal(s.T(), responseutils.Processing, respOO.Issue[0].Details.Coding[0].Display)

	os.Setenv("BB_CLIENT_CERT_FILE", origBBCertPath)
}

func (s *APITestSuite) TestMetadata() {
	req := httptest.NewRequest("GET", "/api/v1/metadata", nil)
	req.TLS = &tls.ConnectionState{}

	handler := http.HandlerFunc(metadata)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)
}

func (s *APITestSuite) TestGetVersion() {
	req := httptest.NewRequest("GET", "/_version", nil)

	handler := http.HandlerFunc(getVersion)
	handler.ServeHTTP(s.rr, req)

	assert.Equal(s.T(), http.StatusOK, s.rr.Code)

	respMap := make(map[string]string)
	err := json.Unmarshal(s.rr.Body.Bytes(), &respMap)
	if err != nil {
		s.T().Error(err.Error())
	}

	assert.Equal(s.T(), "latest", respMap["version"])
}

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}
