package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/CMSgov/bcda-app/bcda/auth"
	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/models"
)

type AlphaAuthPlugin struct{}

type AllClaims struct {
	ACO string `json:"aco"`
	ID  string `json:"id"`
	jwt.StandardClaims
}

// This implementation expects one value in params, an id the API knows this client by in string form
// It returns a single string as well, being the clientID this implementation knows this client by
// NB: Other implementations will probably expect more input, and will certainly return more data
func (p *AlphaAuthPlugin) RegisterClient(params []byte) ([]byte, error) {
	acoUUID, err := GetParamString(params, "clientID")
	if err != nil {
		return nil, err
	}

	// We'll check carefully in this method, because we're returning something to be used as an id
	// Normally, a plugin would treat this value as a black box external key, but this implementation is
	// intimate with the API. So, we're going to protect against accidental bad things
	if len(acoUUID) != 36 {
		return nil, errors.New("you must provide a non-empty string 36 characters in length")
	}

	if matched, err := regexp.MatchString("^[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}$", acoUUID); !matched || err != nil {
		return nil, errors.New("expected a valid UUID string")
	}

	if _, err := getACOFromDB(acoUUID); err != nil {
		return nil, err
	}

	// return the aco UUID as our auth client id. why? because we have to return something that the API / CLI will
	// use as our clientId for all the methods below. We could come up with yet another numbering scheme, or generate
	// more UUIDs, but I can't see a benefit in that. Plus, we will know just looking at the DB that any aco
	// whose client_id matches their UUID was created by this plugin.
	return params, nil
}

func (p *AlphaAuthPlugin) UpdateClient(params []byte) ([]byte, error) {
	return nil, errors.New("not yet implemented")
}

func (p *AlphaAuthPlugin) DeleteClient(params []byte) error {
	return errors.New("not yet implemented")
}

// can treat as a no-op or call RequestAccessToken
func (p *AlphaAuthPlugin) GenerateClientCredentials(params []byte) ([]byte, error) {
	clientID, err := GetParamString(params, "clientID")
	if err != nil {
		return nil, err
	}

	aco, err := getACOFromDB(clientID)
	if err != nil {
		return nil, fmt.Errorf(`no ACO found for client ID %s because %s`, clientID, err)
	}

	if aco.ClientID == "" {
		return nil, fmt.Errorf("ACO %s does not have a registered client", clientID)
	}

	err = p.RevokeClientCredentials([]byte(fmt.Sprintf(`{"clientID":"%s"}`, clientID)))
	if err != nil {
		return nil, fmt.Errorf("unable to revoke existing credentials for ACO %s because %s", clientID, err)
	}

	jwtToken, err := p.RequestAccessToken([]byte(params))
	if err != nil {
		return nil, fmt.Errorf("unable to generate new credentials for ACO %s because %s", clientID, err)
	}
	tokenString, err := jwtToken.SignedString(auth.InitAuthBackend().PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("unable to generate tokenString because %s", err)
	}

	return []byte(fmt.Sprintf(`{"tokenString":"%s"}`, tokenString)), err
}

// look up the active access token associated with id, and call RevokeAccessToken
func (p *AlphaAuthPlugin) RevokeClientCredentials(params []byte) error {
	clientID, err := GetParamString(params, "clientID")
	if err != nil {
		return err
	}

	db := database.GetGORMDbConnection()
	defer func() {
		if err := db.Close(); err != nil {
			log.Infof("error closing db connection in %s because %s", "alpha plugin", err)
		}
	}()

	var aco models.ACO
	err = db.First(&aco, "client_id = ?", clientID).Error
	if err != nil {
		return errors.New("no ACO found for client ID")
	}

	var users []models.User
	db.Find(&users, "aco_id = ?", aco.UUID)
	if len(users) == 0 {
		return errors.New("no users found in client's ACO")
	}

	var (
		userIDs []uuid.UUID
		tokens  []auth.Token
	)
	for _, u := range users {
		userIDs = append(userIDs, u.UUID)
	}

	db.Find(&tokens, "user_id in (?) and active = true", userIDs)
	if len(tokens) == 0 {
		log.Info("No tokens found to revoke for users in client's ACO.")
		return nil
	}

	var errs []string
	revokedCount := 0
	for _, t := range tokens {
		err := revokeAccessTokenByID(t.UUID)
		if err != nil {
			log.Error(err)
			errs = append(errs, err.Error())
		} else {
			revokedCount = revokedCount + 1
		}
	}
	log.Infof("%d token(s) revoked.", revokedCount)
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d token(s) could not be revoked due to errors", len(errs), len(tokens))
	}

	return nil
}

// generate a token for the id (which user? just have a single "user" (alpha2, alpha3, ...) per test cycle?)
// params are currently acoId and ttl; not going to introduce user until we have clear use cases
func (p *AlphaAuthPlugin) RequestAccessToken(params []byte) (jwt.Token, error) {
	backend := auth.InitAuthBackend()
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	jwtToken := jwt.Token{}

	acoUUID, err := GetParamString(params, "clientID")
	if err != nil {
		return jwtToken, err
	}

	aco, err := getACOFromDB(acoUUID)
	if err != nil {
		return jwtToken, err
	}

	// I arbitrarily decided to use the first user. An alternative would be to make a specific user
	// that represents the client. I have no strong opinion here other than not creating stuff in the db
	// unless we're willing to live with it forever.
	var user models.User
	if err = db.First(&user, "aco_id = ?", aco.UUID).Error; err != nil {
		return jwtToken, errors.New("no user found for " + aco.UUID.String())
	}

	ttl, err := GetParamPositiveInt(params, "ttl")
	if err != nil {
		return jwtToken, errors.New("no valid ttl found because " + err.Error())
	}

	tokenUUID := uuid.NewRandom()
	jwtToken = *jwt.New(jwt.SigningMethodRS512)
	jwtToken.Claims = jwt.MapClaims{
		"exp": time.Now().Add(time.Hour * time.Duration(ttl)).Unix(),
		"iat": time.Now().Unix(),
		"sub": user.UUID.String(),
		"aco": aco.UUID.String(),
		"id":  tokenUUID.String(),
	}

	tokenString, err := backend.SignJwtToken(jwtToken)
	if err != nil {
		return jwtToken, err
	}

	token := auth.Token{
		UUID:        tokenUUID,
		UserID:      user.UUID,
		Value:       tokenString, // replaced with hash when saved to db
		Active:      true,
		Token:       jwtToken,
		TokenString: tokenString,
	}

	if err = db.Create(&token).Error; err != nil {
		return jwtToken, err
	}

	return jwtToken, err // really want to return auth.Token here, but first let's get this all working
}

func (p *AlphaAuthPlugin) RevokeAccessToken(tokenString string) error {
	t, err := p.DecodeAccessToken(tokenString)
	if err != nil {
		return err
	}

	if c, ok := t.Claims.(*AllClaims); ok {
		return revokeAccessTokenByID(uuid.Parse(c.ID))
	}

	return errors.New("could not read token claims")
}

func revokeAccessTokenByID(tokenID uuid.UUID) error {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	var token auth.Token
	if db.First(&token, "UUID = ? and active = true", tokenID).RecordNotFound() {
		return gorm.ErrRecordNotFound
	}

	token.Active = false
	db.Save(&token)

	return db.Error
}

func (p *AlphaAuthPlugin) ValidateAccessToken(tokenString string) error {
	t, err := p.DecodeAccessToken(tokenString)
	if err != nil {
		return err
	}

	c := t.Claims.(*AllClaims)

	err = checkRequiredClaims(c)
	if err != nil {
		return err
	}

	err = c.Valid()
	if err != nil {
		return err
	}

	_, err = getACOFromDB(c.ACO)
	if err != nil {
		return err
	}

	b := isActive(t)
	if !b {
		return fmt.Errorf("token with id: %v is not active", c.ID)
	}

	return nil
}

func checkRequiredClaims(claims *AllClaims) error {
	if claims.ExpiresAt == 0 ||
		claims.IssuedAt == 0 ||
		claims.Subject == "" ||
		claims.ACO == "" ||
		claims.ID == "" {
		return fmt.Errorf("missing one or more required claims")
	}
	return nil
}

func isActive(token jwt.Token) bool {
	c := token.Claims.(*AllClaims)

	db := database.GetGORMDbConnection()
	defer database.Close(db)

	return !db.Find(&token, "UUID = ? AND active = ?", c.ID, true).RecordNotFound()
}

func (p *AlphaAuthPlugin) DecodeAccessToken(tokenString string) (jwt.Token, error) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return auth.InitAuthBackend().PublicKey, nil
	}
	t, err := jwt.ParseWithClaims(tokenString, &AllClaims{}, keyFunc)
	if err != nil {
		return jwt.Token{}, err
	}
	return *t, nil
}

func getACOFromDB(acoUUID string) (models.ACO, error) {
	var (
		db  = database.GetGORMDbConnection()
		aco models.ACO
		err error
	)
	defer database.Close(db)

	if db.Find(&aco, "UUID = ?", acoUUID).RecordNotFound() {
		err = errors.New("no ACO record found for " + acoUUID)
	}
	return aco, err
}

func GetParamString(params []byte, name string) (string, error) {
	var (
		j   interface{}
		err error
	)

	if err = json.Unmarshal(params, &j); err != nil {
		return "", err
	}
	paramsMap := j.(map[string]interface{})

	stringForName, ok := paramsMap[name].(string)
	if !ok {
		return "", errors.New("missing or otherwise invalid string value for " + name)
	}

	return stringForName, err
}

func GetParamPositiveInt(params []byte, name string) (int, error) {
	var (
		j   interface{}
		err error
	)

	if err = json.Unmarshal(params, &j); err != nil {
		return -1, err
	}
	paramsMap := j.(map[string]interface{})

	valueForName, ok := paramsMap[name].(float64)
	if !ok {
		return -1, errors.New("missing or otherwise invalid int value for " + name)
	}

	return int(valueForName), err
}
