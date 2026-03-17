package oauth

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/T4cceptor/centian/internal/config"
	"golang.org/x/oauth2"
)

const defaultPendingTTL = 15 * time.Minute

type PendingStatus string

const (
	PendingStatusReady      PendingStatus = "ready"
	PendingStatusInProgress PendingStatus = "in_progress"
	PendingStatusCompleted  PendingStatus = "completed"
	PendingStatusFailed     PendingStatus = "failed"
)

type PendingAuthorization struct {
	ID           string
	State        string
	Status       PendingStatus
	Binding      Binding
	ClientID     string
	ClientSecret string
	Metadata     ResolvedMetadata
	CodeVerifier string
	RedirectURL  string
	AuthURL      string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastError    string
}

type pendingStore struct {
	mu      sync.Mutex
	byID    map[string]*PendingAuthorization
	byState map[string]string
}

func newPendingStore() *pendingStore {
	return &pendingStore{
		byID:    make(map[string]*PendingAuthorization),
		byState: make(map[string]string),
	}
}

func (s *pendingStore) Put(pending *PendingAuthorization) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	s.byID[pending.ID] = pending
	s.byState[pending.State] = pending.ID
}

func (s *pendingStore) GetByID(id string) *PendingAuthorization {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	return s.byID[id]
}

func (s *pendingStore) GetByState(state string) *PendingAuthorization {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	id := s.byState[state]
	if id == "" {
		return nil
	}
	return s.byID[id]
}

func (s *pendingStore) List() []*PendingAuthorization {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	items := make([]*PendingAuthorization, 0, len(s.byID))
	for _, pending := range s.byID {
		if pending != nil {
			items = append(items, pending)
		}
	}
	return items
}

func (s *pendingStore) gcLocked() {
	now := time.Now()
	for id, pending := range s.byID {
		if pending == nil || now.After(pending.ExpiresAt) {
			if pending != nil {
				delete(s.byState, pending.State)
			}
			delete(s.byID, id)
		}
	}
}

type AuthorizationRequiredError struct {
	Binding   Binding
	AuthURL   string
	StatusURL string
	PendingID string
	Reason    string
}

func (e *AuthorizationRequiredError) Error() string {
	return fmt.Sprintf(
		"downstream oauth authorization required for %s/%s: visit %s",
		e.Binding.Gateway,
		e.Binding.Server,
		e.AuthURL,
	)
}

type Manager struct {
	publicBaseURL string
	tokenStore    *EncryptedTokenStore
	pending       *pendingStore
	httpClient    *http.Client
	onAuthorized  func(Binding)
	onRequired    func(Binding, string)
	now           func() time.Time
}

func NewManager(publicBaseURL string, onAuthorized func(Binding), onRequired func(Binding, string)) (*Manager, error) {
	keyManager, err := NewDefaultMasterKeyManager()
	if err != nil {
		return nil, err
	}
	tokenStore, err := NewDefaultEncryptedTokenStore(keyManager)
	if err != nil {
		return nil, err
	}
	return &Manager{
		publicBaseURL: trimTrailingSlash(publicBaseURL),
		tokenStore:    tokenStore,
		pending:       newPendingStore(),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		onAuthorized:  onAuthorized,
		onRequired:    onRequired,
		now:           time.Now,
	}, nil
}

func (m *Manager) NewTransport(
	base http.RoundTripper,
	binding Binding,
	downstreamURL string,
	oauthConfig *config.OAuthConfig,
	headers map[string]string,
) http.RoundTripper {
	return &Transport{
		Base:          base,
		Manager:       m,
		Binding:       binding,
		DownstreamURL: downstreamURL,
		Config:        oauthConfig,
		Headers:       headers,
	}
}

func (m *Manager) LoadToken(binding Binding) (*StoredToken, error) {
	return m.tokenStore.Load(binding)
}

func (m *Manager) SaveToken(binding Binding, token *StoredToken) error {
	return m.tokenStore.Save(binding, token)
}

func (m *Manager) DeleteToken(binding Binding) error {
	return m.tokenStore.Delete(binding)
}

func (m *Manager) CreatePending(binding Binding, clientID, clientSecret string, metadata ResolvedMetadata, verifier string) (*PendingAuthorization, error) {
	id, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	state, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	redirectURL := m.publicBaseURL + "/oauth/callback"
	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   metadata.AuthorizationEndpoint,
			TokenURL:  metadata.TokenEndpoint,
			AuthStyle: authMethodToStyle(metadata.ClientAuthMethod),
		},
		Scopes: append([]string(nil), metadata.Scopes...),
	}
	authURL := cfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("resource", metadata.Resource),
	)
	pending := &PendingAuthorization{
		ID:           id,
		State:        state,
		Status:       PendingStatusReady,
		Binding:      binding,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Metadata:     metadata,
		CodeVerifier: verifier,
		RedirectURL:  redirectURL,
		AuthURL:      authURL,
		CreatedAt:    m.now().UTC(),
		ExpiresAt:    m.now().UTC().Add(defaultPendingTTL),
	}
	m.pending.Put(pending)
	if m.onRequired != nil {
		m.onRequired(binding, m.StartURL(id))
	}
	return pending, nil
}

func (m *Manager) PendingForBinding(binding Binding) *PendingAuthorization {
	if m == nil {
		return nil
	}
	for _, pending := range m.pending.List() {
		if pending == nil {
			continue
		}
		if pending.Binding == binding {
			return pending
		}
	}
	return nil
}

func (m *Manager) EnsurePending(binding Binding, clientID, clientSecret string, metadata ResolvedMetadata) (*PendingAuthorization, bool, error) {
	if pending := m.PendingForBinding(binding); pending != nil {
		switch pending.Status {
		case PendingStatusReady, PendingStatusInProgress:
			return pending, true, nil
		}
	}

	verifier := oauth2.GenerateVerifier()
	pending, err := m.CreatePending(binding, clientID, clientSecret, metadata, verifier)
	if err != nil {
		return nil, false, err
	}
	return pending, false, nil
}

func (m *Manager) StartURL(id string) string {
	values := url.Values{}
	values.Set("id", id)
	return m.publicBaseURL + "/oauth/start?" + values.Encode()
}

func (m *Manager) StatusURL(id string) string {
	values := url.Values{}
	values.Set("id", id)
	return m.publicBaseURL + "/oauth/status?" + values.Encode()
}

func (m *Manager) RegisterRoutes(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/oauth/start", m.handleStart)
	mux.HandleFunc("/oauth/status", m.handleStatus)
	mux.HandleFunc("/oauth/callback", m.handleCallback)
}

func (m *Manager) handleStart(w http.ResponseWriter, r *http.Request) {
	pending := m.pending.GetByID(r.URL.Query().Get("id"))
	if pending == nil {
		http.Error(w, "OAuth flow not found or expired", http.StatusNotFound)
		return
	}
	pending.Status = PendingStatusInProgress
	http.Redirect(w, r, pending.AuthURL, http.StatusFound)
}

func (m *Manager) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<html><body><h1>Pending Downstream OAuth Flows</h1><ul>")
		for _, pending := range m.pending.List() {
			_, _ = fmt.Fprintf(
				w,
				`<li>%s/%s [%s] <a href="%s">start</a> <a href="%s">status</a></li>`,
				html.EscapeString(pending.Binding.Gateway),
				html.EscapeString(pending.Binding.Server),
				html.EscapeString(string(pending.Status)),
				html.EscapeString(m.StartURL(pending.ID)),
				html.EscapeString(m.StatusURL(pending.ID)),
			)
		}
		_, _ = fmt.Fprint(w, "</ul></body></html>")
		return
	}

	pending := m.pending.GetByID(r.URL.Query().Get("id"))
	if pending == nil {
		http.Error(w, "OAuth flow not found or expired", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(
		w,
		"<html><body><h1>Downstream OAuth</h1><p>Server: %s/%s</p><p>Status: %s</p><p>%s</p></body></html>",
		html.EscapeString(pending.Binding.Gateway),
		html.EscapeString(pending.Binding.Server),
		html.EscapeString(string(pending.Status)),
		html.EscapeString(pending.LastError),
	)
}

func (m *Manager) handleCallback(w http.ResponseWriter, r *http.Request) {
	pending := m.pending.GetByState(r.URL.Query().Get("state"))
	if pending == nil {
		http.Error(w, "OAuth flow not found or expired", http.StatusNotFound)
		return
	}
	if errText := r.URL.Query().Get("error"); errText != "" {
		pending.Status = PendingStatusFailed
		pending.LastError = errText
		http.Error(w, errText, http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	cfg := oauth2.Config{
		ClientID:     pending.ClientID,
		ClientSecret: pending.ClientSecret,
		RedirectURL:  pending.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   pending.Metadata.AuthorizationEndpoint,
			TokenURL:  pending.Metadata.TokenEndpoint,
			AuthStyle: authMethodToStyle(pending.Metadata.ClientAuthMethod),
		},
		Scopes: append([]string(nil), pending.Metadata.Scopes...),
	}
	clientCtx := context.WithValue(r.Context(), oauth2.HTTPClient, m.httpClient)
	token, err := cfg.Exchange(
		clientCtx,
		code,
		oauth2.VerifierOption(pending.CodeVerifier),
		oauth2.SetAuthURLParam("resource", pending.Metadata.Resource),
	)
	if err != nil {
		pending.Status = PendingStatusFailed
		pending.LastError = err.Error()
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	if err := m.SaveToken(pending.Binding, tokenFromOAuth(token, &StoredToken{
		Resource:              pending.Metadata.Resource,
		Scopes:                pending.Metadata.Scopes,
		Issuer:                pending.Metadata.Issuer,
		AuthorizationEndpoint: pending.Metadata.AuthorizationEndpoint,
		TokenEndpoint:         pending.Metadata.TokenEndpoint,
		ClientAuthMethod:      pending.Metadata.ClientAuthMethod,
	})); err != nil {
		pending.Status = PendingStatusFailed
		pending.LastError = err.Error()
		http.Error(w, "failed to persist token", http.StatusInternalServerError)
		return
	}
	pending.Status = PendingStatusCompleted
	pending.LastError = ""
	if m.onAuthorized != nil {
		go m.onAuthorized(pending.Binding)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(
		w,
		"<html><body><h1>Authorization complete</h1><p>Server: %s/%s</p><p>You can return to your MCP client.</p></body></html>",
		html.EscapeString(pending.Binding.Gateway),
		html.EscapeString(pending.Binding.Server),
	)
}

func trimTrailingSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}
