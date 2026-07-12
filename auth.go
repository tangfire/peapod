package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// login renders the login page (GET) or processes a form-based login (POST).
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := a.currentUser(r); ok {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		a.serveFrontend(w, r, loginTemplate)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/login?error=bad_request", http.StatusFound)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		user, err := a.authenticate(r.Context(), username, password)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
			return
		}
		a.setSession(w, r, user)
		http.Redirect(w, r, "/", http.StatusFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, err := a.authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		http.Error(w, "用户名、邮箱或密码不正确", http.StatusUnauthorized)
		return
	}
	a.setSession(w, r, user)
	writeJSON(w, map[string]any{"user": user})
}

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearSessionCookie(w, r)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// clearSessionCookie invalidates the session cookie by expiring it immediately.
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.currentUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authUserContextKey{}, user)))
	}
}

func (a *App) authenticate(ctx context.Context, username, password string) (AuthUser, error) {
	if a.store != nil {
		return a.store.Authenticate(ctx, username, password)
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(a.cfg.Password)) != 1 {
		return AuthUser{}, errors.New("invalid password")
	}
	return AuthUser{Username: "legacy-admin", DisplayName: "管理员", Role: "admin", Active: true, Legacy: true}, nil
}

func (a *App) setSession(w http.ResponseWriter, r *http.Request, user AuthUser) {
	payload := sessionPayload{
		Expires:     time.Now().Add(24 * time.Hour).Unix(),
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Role:        user.Role,
		Legacy:      user.Legacy,
	}
	token := a.signSession(payload)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", MaxAge: 86400, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) currentUser(r *http.Request) (AuthUser, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return AuthUser{}, false
	}
	payload, ok := a.parseSession(cookie.Value)
	if !ok {
		return AuthUser{}, false
	}
	if payload.Expires <= time.Now().Unix() {
		return AuthUser{}, false
	}
	if a.store == nil || payload.Legacy {
		return AuthUser{ID: payload.UserID, Username: payload.Username, DisplayName: payload.DisplayName, Email: payload.Email, Role: payload.Role, Active: true, Legacy: payload.Legacy}, true
	}
	user, err := a.store.GetUser(r.Context(), payload.UserID)
	if err != nil || !user.Active {
		return AuthUser{}, false
	}
	return user, true
}

func (a *App) signSession(payload sessionPayload) string {
	body, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) parseSession(token string) (sessionPayload, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sessionPayload{}, false
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.SessionSecret))
	_, _ = mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return sessionPayload{}, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionPayload{}, false
	}
	var payload sessionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return sessionPayload{}, false
	}
	return payload, true
}
