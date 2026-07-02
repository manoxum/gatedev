package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "bindnet_session"
	sessionDuration   = 12 * time.Hour
)

type session struct {
	Username string `json:"username"`
	Exp      int64  `json:"exp"`
}

// issueToken cria um token assinado (payload base64url + assinatura
// HMAC-SHA256) - um substituto minimalista de JWT sem depender de
// bibliotecas externas, guardado num cookie httpOnly.
func issueToken(username string, secret []byte) (string, error) {
	s := session{Username: username, Exp: time.Now().Add(sessionDuration).Unix()}
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return payloadB64 + "." + sign(payloadB64, secret), nil
}

func validateToken(token string, secret []byte) (string, bool) {
	partes := strings.SplitN(token, ".", 2)
	if len(partes) != 2 {
		return "", false
	}
	if !hmac.Equal([]byte(sign(partes[0], secret)), []byte(partes[1])) {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(partes[0])
	if err != nil {
		return "", false
	}
	var s session
	if err := json.Unmarshal(payload, &s); err != nil {
		return "", false
	}
	if time.Now().Unix() > s.Exp {
		return "", false
	}
	return s.Username, true
}

func sign(payloadB64 string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func registerAuthRoutes(mux *http.ServeMux, admin *administrator, audit *auditClient) {
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var c credentials
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "corpo invalido", http.StatusBadRequest)
			return
		}
		if c.Username != admin.Username || !admin.validPassword(c.Password) {
			http.Error(w, "usuario ou senha invalidos", http.StatusUnauthorized)
			return
		}
		token, err := issueToken(admin.Username, admin.secret())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(sessionDuration.Seconds()),
		})
		audit.record(r.Context(), "login", admin.Username, nil)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		username, _ := sessionUser(r, admin)
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
		audit.record(r.Context(), "logout", username, nil)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		username, ok := sessionUser(r, admin)
		if !ok {
			http.Error(w, "nao autenticado", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"username": username})
	})
}

func sessionUser(r *http.Request, admin *administrator) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	return validateToken(cookie.Value, admin.secret())
}

// requireSession protege as rotas de negocio - so libera a requisicao
// se o cookie de sessao emitido em /api/auth/login for valido.
func requireSession(admin *administrator, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessionUser(r, admin); !ok {
			http.Error(w, "nao autenticado", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
