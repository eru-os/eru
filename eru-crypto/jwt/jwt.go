package jwt

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"sync"
	"time"
)

var (
	jwkRefresherOnce sync.Once
	jwkRefresher     *jwk.AutoRefresh
	jwkConfigured    sync.Map
	jwkLastForced    sync.Map
)

func fetchJWKSet(ctx context.Context, jwkurl string) (jwk.Set, error) {
	jwkRefresherOnce.Do(func() {
		jwkRefresher = jwk.NewAutoRefresh(context.Background())
	})
	if _, loaded := jwkConfigured.LoadOrStore(jwkurl, true); !loaded {
		jwkRefresher.Configure(jwkurl, jwk.WithMinRefreshInterval(5*time.Minute))
	}
	return jwkRefresher.Fetch(ctx, jwkurl)
}

func refreshJWKSet(ctx context.Context, jwkurl string) (jwk.Set, error) {
	now := time.Now()
	if lastI, ok := jwkLastForced.Load(jwkurl); ok {
		if last, lok := lastI.(time.Time); lok && now.Sub(last) < time.Minute {
			return jwkRefresher.Fetch(ctx, jwkurl)
		}
	}
	jwkLastForced.Store(jwkurl, now)
	return jwkRefresher.Refresh(ctx, jwkurl)
}

func lookupRawKeyJWK(ctx context.Context, set jwk.Set, kid string) (interface{}, bool, error) {
	for it := set.Iterate(context.Background()); it.Next(context.Background()); {
		pair := it.Pair()
		key := pair.Value.(jwk.Key)
		if kid == key.KeyID() {
			var rawkey interface{}
			if err := key.Raw(&rawkey); err != nil {
				err = errors.New(fmt.Sprint("Failed to create public key - ", err.Error()))
				logs.WithContext(ctx).Error(err.Error())
				return nil, false, err
			}
			return rawkey, true, nil
		}
	}
	return nil, false, nil
}

func FetchKeyJWK(ctx context.Context, kid string, jwkurl string) (interface{}, error) {
	set, err := fetchJWKSet(ctx, jwkurl)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to parse JWK: ", err))
		return nil, errors.New("Failed to parse JWK")
	}
	rawkey, found, err := lookupRawKeyJWK(ctx, set, kid)
	if err != nil {
		return nil, err
	}
	if found {
		return rawkey, nil
	}
	set, err = refreshJWKSet(ctx, jwkurl)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to refresh JWK: ", err))
		return nil, errors.New("Failed to parse JWK")
	}
	rawkey, found, err = lookupRawKeyJWK(ctx, set, kid)
	if err != nil {
		return nil, err
	}
	if found {
		return rawkey, nil
	}
	err = errors.New(fmt.Sprint("kid ", kid, " not found"))
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func DecryptTokenJWK(ctx context.Context, strToken string, jwkurl string) (objToken interface{}, err error) {

	tokenObj, err := jwt.Parse(strToken, func(token *jwt.Token) (interface{}, error) {
		// TODO Don't forget to validate the alg is what you expect:

		//if token.Method.Alg() != string(secret.Alg) {
		//	return nil, fmt.Errorf("invalid token algorithm provided wanted (%s) got (%s)", secret.Alg, token.Method.Alg())
		//}
		//
		//if secret.JwkURL != "" {
		//	return secret.JwkKey, nil
		//}

		//switch secret.Alg {
		//case config.RS256:
		//	return jwt.ParseRSAPublicKeyFromPEM([]byte(secret.PublicKey))
		//case config.HS256, "":
		//		return []byte(secret.Secret), nil
		//	default:
		//		return nil, fmt.Errorf("invalid token algorithm (%s) provided", secret.Alg)
		//	}
		kid := ""
		if kidStr, kidOk := token.Header["kid"].(string); kidOk {
			kid = kidStr
		}
		key, err := FetchKeyJWK(ctx, kid, jwkurl)
		return key, err
	})

	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	//_=tokenObj

	// Get the claims
	if claims, ok := tokenObj.Claims.(jwt.MapClaims); ok && tokenObj.Valid {
		if err := claims.Valid(); err != nil {
			return nil, err
		}
		obj := make(map[string]interface{}, len(claims))
		for key, val := range claims {
			obj[key] = val
		}

		/*
			if len(secret.Issuer) > 0 {
				c, ok := claims["iss"]
				if !ok {
					return nil, errors.New("claim (iss) not provided in token")
				}
				if err := verifyClaims(c, secret.Issuer); err != nil {
					return nil, err
				}
			}

			if len(secret.Audience) > 0 {
				c, ok := claims["aud"]
				if !ok {
					return nil, errors.New("claim (aud) not provided in token")
				}
				if err := verifyClaims(c, secret.Audience); err != nil {
					return nil, err
				}
			}
		*/
		return obj, nil
	}
	err = errors.New("AUTH: JWT token could not be verified")
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}
func CreateJWT(ctx context.Context, privateKeyStr string, claimsMap map[string]interface{}, claimsHeader map[string]interface{}) (tokenString string, err error) {
	token := jwt.New(jwt.SigningMethodRS256)

	var privateKey *rsa.PrivateKey

	if privateKey, err = jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyStr)); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	headers := token.Header

	if claimsMap != nil {
		for k, v := range claimsMap {
			claims[k] = v
		}
	}
	if claimsHeader != nil {
		for k, v := range claimsHeader {
			headers[k] = v
		}
	}
	tokenString, err = token.SignedString(privateKey)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
	}
	return
}

// VerifyTokenWithPublicKey validates a token against a PEM public key held locally, for a service
// that issued the token itself and so has no reason to fetch its own key set over http. The signing
// method is pinned to RSA: without that check a token could name "none" and verify with no key at
// all, which is the oldest trick against a jwt library.
func VerifyTokenWithPublicKey(ctx context.Context, strToken string, publicKeyStr string) (claims map[string]interface{}, err error) {
	publicKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(publicKeyStr))
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	tokenObj, err := jwt.Parse(strToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method : %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}

	tokenClaims, claimsOk := tokenObj.Claims.(jwt.MapClaims)
	if !claimsOk || !tokenObj.Valid {
		err = errors.New("token is not valid")
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
	if err = tokenClaims.Valid(); err != nil {
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}

	claims = make(map[string]interface{}, len(tokenClaims))
	for key, value := range tokenClaims {
		claims[key] = value
	}
	return claims, nil
}

// TokenKid reads the kid from a token header without validating the token, so the right key can be
// looked up before verification.
func TokenKid(ctx context.Context, strToken string) string {
	parser := jwt.Parser{SkipClaimsValidation: true}
	token, _, err := parser.ParseUnverified(strToken, jwt.MapClaims{})
	if err != nil {
		logs.WithContext(ctx).Info(err.Error())
		return ""
	}
	if kid, ok := token.Header["kid"].(string); ok {
		return kid
	}
	return ""
}
