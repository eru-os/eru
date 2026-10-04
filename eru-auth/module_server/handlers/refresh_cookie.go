package handlers

import (
	"net/http"

	"github.com/eru-os/eru/eru-auth/auth"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// The refresh token is the long lived credential in the login api's response. Handing it to
// javascript means an xss bug becomes a lasting account takeover, so when the auth is configured
// for it the token travels in an http only cookie instead - readable by the browser's network
// stack, never by page script.
//
// The cookie is issued on the wildcard parent domain so it reaches every api subdomain of a
// project, and defaults to SameSite=Lax, which keeps it off cross site posts and is what stops
// another origin driving the refresh endpoint.

func refreshCookieConfig(r *http.Request, authObjI auth.AuthI) auth.RefreshCookieConfig {
	if authObjI == nil {
		return auth.RefreshCookieConfig{}
	}
	return authObjI.OAuthServer(r.Context()).RefreshCookie
}

// setRefreshCookie writes the cookie when the auth asks for it. MaxAge follows the configured
// refresh lifetime so the cookie and the token it carries expire together.
func setRefreshCookie(w http.ResponseWriter, r *http.Request, authObjI auth.AuthI, refreshToken string) {
	cookie := refreshCookieConfig(r, authObjI)
	if !cookie.Enabled || refreshToken == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookie.CookieName(),
		Value:    refreshToken,
		Domain:   cookie.Domain,
		Path:     cookie.CookiePath(),
		MaxAge:   authObjI.OAuthServer(r.Context()).RefreshTokenLifespanSeconds(),
		HttpOnly: true,
		Secure:   isRequestSecure(r),
		SameSite: cookie.SameSiteMode(),
	})
}

// clearRefreshCookie expires the cookie. It runs whether or not anything could be revoked, so a
// browser is never left holding a credential it cannot use.
func clearRefreshCookie(w http.ResponseWriter, r *http.Request, authObjI auth.AuthI) {
	cookie := refreshCookieConfig(r, authObjI)
	if !cookie.Enabled {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookie.CookieName(),
		Value:    "",
		Domain:   cookie.Domain,
		Path:     cookie.CookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isRequestSecure(r),
		SameSite: cookie.SameSiteMode(),
	})
}

// refreshTokenFromRequest prefers what the caller sent explicitly and falls back to the cookie, so
// an existing client that posts the token keeps working unchanged while a browser can stop holding
// one at all.
func refreshTokenFromRequest(r *http.Request, authObjI auth.AuthI, postedRefreshToken string) string {
	if postedRefreshToken != "" {
		return postedRefreshToken
	}
	cookie := refreshCookieConfig(r, authObjI)
	if !cookie.Enabled {
		return ""
	}
	refreshCookie, err := r.Cookie(cookie.CookieName())
	if err != nil {
		logs.WithContext(r.Context()).Info("no refresh cookie on the request")
		return ""
	}
	return refreshCookie.Value
}

// setRefreshCookieFromResult handles the refresh paths, whose result is an interface because an
// external backend could return anything. A rotated token has to reach the cookie, or the next
// refresh would present the retired one and trip reuse detection.
func setRefreshCookieFromResult(w http.ResponseWriter, r *http.Request, authObjI auth.AuthI, result interface{}) interface{} {
	tokens, tokensOk := result.(auth.LoginSuccess)
	if !tokensOk {
		return result
	}
	setRefreshCookie(w, r, authObjI, tokens.RefreshToken)
	return hideRefreshToken(r, authObjI, tokens)
}

// hideRefreshToken blanks the token in the response body once every caller reads the cookie. It is
// separate from issuing the cookie because turning it on is a breaking change for anyone still
// reading the body.
func hideRefreshToken(r *http.Request, authObjI auth.AuthI, tokens auth.LoginSuccess) auth.LoginSuccess {
	cookie := refreshCookieConfig(r, authObjI)
	if cookie.Enabled && cookie.OmitFromBody {
		tokens.RefreshToken = ""
	}
	return tokens
}
