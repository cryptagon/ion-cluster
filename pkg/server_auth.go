package cluster

import (
	// pprof
	"errors"
	"fmt"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	log "github.com/pion/ion-log"
)

var (
	errorTokenClaimsInvalid = fmt.Errorf("Token claims invalid: must have SID")
)

type authToken struct {
	SID string `json:"sid"`
	*jwt.StandardClaims
}

func (t *authToken) Valid() error {

	if t.SID == "" {
		return errorTokenClaimsInvalid
	}

	if t.StandardClaims != nil {
		return t.StandardClaims.Valid()
	}
	return nil
}

func authGetAndValidateToken(config AuthConfig, r *http.Request) (*authToken, error) {
	vars := r.URL.Query()
	log.Debugf("Authenticating token")
	tokenParam := vars["access_token"]
	if tokenParam == nil || len(tokenParam) < 1 {
		return nil, errors.New("no token")
	}

	tokenStr := tokenParam[0]

	log.Debugf("checking claims on token %v", tokenStr)
	token, err := jwt.ParseWithClaims(tokenStr, &authToken{}, config.keyFunc)
	if err != nil {
		return nil, err
	}
	return token.Claims.(*authToken), nil
}
