package a2a

// Test doubles and httptest fixtures for the federation end-to-end tests
// (T17). Repos are in-memory fakes — the PG-backed repo contract
// is covered by internal/data integration tests (T4); these E2E tests focus on
// the cross-layer flow: FederationUsecase governance chain + real
// FederationRemoteInvoker + real HTTP against a mock remote A2A endpoint.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ---------- SSRF test hook ----------

// allowAllRemoteIPs swaps the SSRF block predicate so tests can reach
// httptest servers on loopback. Restored on test cleanup. E2E tests must NOT
// run in parallel: parallel tests in this package park until sequential tests
// finish, so the swap never races them.
func allowAllRemoteIPs(t *testing.T) {
	t.Helper()
	old := isBlockedIPFn
	isBlockedIPFn = func(net.IP) bool { return false }
	t.Cleanup(func() { isBlockedIPFn = old })
}

// ---------- fake FederationOrgRepo ----------

type fakeFedOrgRepo struct {
	mu       sync.Mutex
	byID     map[string]a2abiz.FederationOrg
	byDomain map[string]string
	order    []string // insertion order of org IDs (survives caller-supplied IDs)
	seq      int
}

func newFakeFedOrgRepo() *fakeFedOrgRepo {
	return &fakeFedOrgRepo{byID: map[string]a2abiz.FederationOrg{}, byDomain: map[string]string{}}
}

func (r *fakeFedOrgRepo) UpsertOrg(_ context.Context, org a2abiz.FederationOrg) (a2abiz.FederationOrg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byDomain[org.Domain]; ok {
		existing := r.byID[id]
		existing.Name = org.Name
		existing.PublicBaseURL = org.PublicBaseURL
		if org.TrustLevel != "" {
			existing.TrustLevel = org.TrustLevel
		}
		if org.AuthType != "" {
			existing.AuthType = org.AuthType
		}
		if org.AuthConfigJSON != "" {
			existing.AuthConfigJSON = org.AuthConfigJSON
		}
		if org.Status != "" {
			existing.Status = org.Status
		}
		existing.UpdatedAt = time.Now()
		r.byID[id] = existing
		return existing, nil
	}
	r.seq++
	if org.ID == "" {
		org.ID = fmt.Sprintf("org-%d", r.seq)
	}
	now := time.Now()
	org.JoinedAt = now
	org.UpdatedAt = now
	if org.TrustLevel == "" {
		org.TrustLevel = a2abiz.TrustLevelNeutral
	}
	if org.Status == "" {
		org.Status = a2abiz.OrgStatusActive
	}
	r.byID[org.ID] = org
	r.byDomain[org.Domain] = org.ID
	r.order = append(r.order, org.ID)
	return org, nil
}

func (r *fakeFedOrgRepo) GetOrg(_ context.Context, id string) (a2abiz.FederationOrg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	org, ok := r.byID[id]
	if !ok {
		return a2abiz.FederationOrg{}, apierror.NotFound(apierror.DomainA2AFed, "federation org %s not found", id)
	}
	return org, nil
}

func (r *fakeFedOrgRepo) ListOrgs(_ context.Context) ([]a2abiz.FederationOrg, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]a2abiz.FederationOrg, 0, len(r.byID))
	for _, id := range r.order {
		if org, ok := r.byID[id]; ok {
			out = append(out, org)
		}
	}
	return out, nil
}

func (r *fakeFedOrgRepo) UpdateOrgTrust(_ context.Context, id, trustLevel string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	org, ok := r.byID[id]
	if !ok {
		return apierror.NotFound(apierror.DomainA2AFed, "federation org %s not found", id)
	}
	org.TrustLevel = trustLevel
	org.UpdatedAt = time.Now()
	r.byID[id] = org
	return nil
}

func (r *fakeFedOrgRepo) DeleteOrg(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	org, ok := r.byID[id]
	if !ok {
		return apierror.NotFound(apierror.DomainA2AFed, "federation org %s not found", id)
	}
	delete(r.byID, id)
	delete(r.byDomain, org.Domain)
	for i, oid := range r.order {
		if oid == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

// ---------- fake FederationPolicyRepo ----------

type fakeFedPolicyRepo struct {
	mu     sync.Mutex
	byID   map[string]a2abiz.FederationPolicy
	byPair map[string]string
	seq    int
}

func newFakeFedPolicyRepo() *fakeFedPolicyRepo {
	return &fakeFedPolicyRepo{byID: map[string]a2abiz.FederationPolicy{}, byPair: map[string]string{}}
}

func fedPairKey(callerOrgID, calleeOrgID string) string { return callerOrgID + "\x00" + calleeOrgID }

func (r *fakeFedPolicyRepo) UpsertPolicy(_ context.Context, p a2abiz.FederationPolicy) (a2abiz.FederationPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fedPairKey(p.CallerOrgID, p.CalleeOrgID)
	if id, ok := r.byPair[key]; ok {
		existing := r.byID[id]
		existing.Action = p.Action
		existing.MaxPerMin = p.MaxPerMin
		existing.DailyQuota = p.DailyQuota
		existing.UpdatedAt = time.Now()
		r.byID[id] = existing
		return existing, nil
	}
	r.seq++
	if p.ID == "" {
		p.ID = fmt.Sprintf("pol-%d", r.seq)
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	r.byID[p.ID] = p
	r.byPair[key] = p.ID
	return p, nil
}

func (r *fakeFedPolicyRepo) GetPolicy(_ context.Context, callerOrgID, calleeOrgID string) (a2abiz.FederationPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byPair[fedPairKey(callerOrgID, calleeOrgID)]
	if !ok {
		return a2abiz.FederationPolicy{}, apierror.NotFound(apierror.DomainA2AFed, "federation policy %s -> %s not found", callerOrgID, calleeOrgID)
	}
	return r.byID[id], nil
}

func (r *fakeFedPolicyRepo) ListPolicies(_ context.Context) ([]a2abiz.FederationPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]a2abiz.FederationPolicy, 0, len(r.byID))
	for i := 1; i <= r.seq; i++ {
		if p, ok := r.byID[fmt.Sprintf("pol-%d", i)]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeFedPolicyRepo) DeletePolicy(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return apierror.NotFound(apierror.DomainA2AFed, "federation policy %s not found", id)
	}
	delete(r.byID, id)
	delete(r.byPair, fedPairKey(p.CallerOrgID, p.CalleeOrgID))
	return nil
}

// ---------- fake FederationAuditRepo ----------

type fakeFedAuditRepo struct {
	mu         sync.Mutex
	entries    []a2abiz.FederationAuditLog
	failCreate bool
}

func (r *fakeFedAuditRepo) CreateAudit(_ context.Context, entry a2abiz.FederationAuditLog) (a2abiz.FederationAuditLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failCreate {
		return a2abiz.FederationAuditLog{}, errors.New("db unavailable (test)")
	}
	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now
	r.entries = append(r.entries, entry)
	return entry, nil
}

func (r *fakeFedAuditRepo) UpdateAuditResult(_ context.Context, id, status string, latencyMs int64, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.ID == id {
			r.entries[i].Status = status
			r.entries[i].LatencyMs = latencyMs
			r.entries[i].ErrorMessage = errMsg
			r.entries[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return apierror.NotFound(apierror.DomainA2AFed, "federation audit %s not found", id)
}

func (r *fakeFedAuditRepo) ListAudits(_ context.Context, filter a2abiz.FederationAuditFilter) ([]a2abiz.FederationAuditLog, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matched := make([]a2abiz.FederationAuditLog, 0, len(r.entries))
	for _, e := range r.entries {
		if filter.CallerOrgID != "" && e.CallerOrgID != filter.CallerOrgID {
			continue
		}
		if filter.CalleeOrgID != "" && e.CalleeOrgID != filter.CalleeOrgID {
			continue
		}
		if filter.Decision != "" && e.Decision != filter.Decision {
			continue
		}
		if filter.Status != "" && e.Status != filter.Status {
			continue
		}
		matched = append(matched, e)
	}
	total := len(matched)
	if filter.Offset > 0 {
		if filter.Offset >= len(matched) {
			matched = nil
		} else {
			matched = matched[filter.Offset:]
		}
	}
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

func (r *fakeFedAuditRepo) CountCallsSince(_ context.Context, callerOrgID, calleeOrgID string, since time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.entries {
		if e.CallerOrgID == callerOrgID && e.CalleeOrgID == calleeOrgID && e.Decision == a2abiz.DecisionAllowed && !e.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

// snapshot returns a copy of all audit entries for assertions.
func (r *fakeFedAuditRepo) snapshot() []a2abiz.FederationAuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]a2abiz.FederationAuditLog, len(r.entries))
	copy(out, r.entries)
	return out
}

// ---------- fake RemoteAgentLister ----------

type fakeRemoteLister struct {
	mu     sync.Mutex
	agents []a2abiz.RemoteAgent
}

func (r *fakeRemoteLister) add(agent a2abiz.RemoteAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents = append(r.agents, agent)
}

func (r *fakeRemoteLister) ListRemoteAgents(_ context.Context, workspace string) ([]a2abiz.RemoteAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]a2abiz.RemoteAgent, 0, len(r.agents))
	for _, a := range r.agents {
		if workspace == "" || a.Workspace == workspace {
			out = append(out, a)
		}
	}
	return out, nil
}

// ---------- card sync no-op ports (not exercised by E2E criteria) ----------

type noopCardDiscoverer struct{}

func (noopCardDiscoverer) DiscoverRemoteCard(_ context.Context, _ a2abiz.RemoteCardDiscoverInput) (a2abiz.AgentCard, error) {
	return a2abiz.AgentCard{}, nil
}

type noopCardWriter struct{}

func (noopCardWriter) UpdateRemoteAgentCard(_ context.Context, _ string, _ a2abiz.AgentCard) error {
	return nil
}

// ---------- E2E fixture (mirrors cmd/admin/wire.go federation providers) ----------

type federationE2EFixture struct {
	uc       *a2abiz.FederationUsecase
	orgs     *fakeFedOrgRepo
	policies *fakeFedPolicyRepo
	audits   *fakeFedAuditRepo
	remotes  *fakeRemoteLister
}

func newFederationE2EFixture(t *testing.T) *federationE2EFixture {
	t.Helper()
	lg := loggateway.NewNoop()
	ctx := context.Background()
	orgs := newFakeFedOrgRepo()
	policies := newFakeFedPolicyRepo()
	audits := &fakeFedAuditRepo{}
	remotes := &fakeRemoteLister{}
	engine := a2abiz.NewPolicyEngine(policies, lg)
	if err := engine.Load(ctx); err != nil {
		t.Fatalf("load policy engine: %v", err)
	}
	factory := func(maxPerMin int) a2abiz.Limiter {
		return a2abiz.NewLimiter(a2abiz.LimiterConfig{WindowSize: time.Minute, MaxInvokes: maxPerMin, KeyPrefix: "test:fed:"}, nil, lg)
	}
	gov := &a2abiz.FederationGovernance{
		Trust:  a2abiz.NewTrustManager(lg),
		Policy: engine,
		Quota:  a2abiz.NewQuotaChecker(engine, audits, factory, lg),
		Audit:  a2abiz.NewAuditLogger(audits, lg),
	}
	executor := NewFederationRemoteInvoker(lg)
	uc := a2abiz.NewFederationUsecase(orgs, gov,
		a2abiz.NewDirectory(orgs, remotes),
		a2abiz.NewAgentCardSync(remotes, noopCardDiscoverer{}, noopCardWriter{}, lg),
		remotes, executor, nil)
	return &federationE2EFixture{uc: uc, orgs: orgs, policies: policies, audits: audits, remotes: remotes}
}

// registerOrg registers one active org with the given trust level.
func (f *federationE2EFixture) registerOrg(t *testing.T, name, domain, trustLevel string) a2abiz.FederationOrg {
	t.Helper()
	org, err := f.uc.RegisterOrg(context.Background(), a2abiz.FederationOrg{
		Name:          name,
		Domain:        domain,
		PublicBaseURL: "https://a2a." + domain,
		TrustLevel:    trustLevel,
		Status:        a2abiz.OrgStatusActive,
	})
	if err != nil {
		t.Fatalf("register org %s: %v", domain, err)
	}
	return org
}

// addRemote registers one enabled remote agent under the org in workspace ws1.
func (f *federationE2EFixture) addRemote(orgID, agentID, remoteURL, authType, authConfigJSON string, capabilities ...string) {
	caps := make([]a2abiz.Capability, 0, len(capabilities))
	for _, c := range capabilities {
		caps = append(caps, a2abiz.Capability{Name: c})
	}
	f.remotes.add(a2abiz.RemoteAgent{
		ID:             agentID,
		Workspace:      "ws1",
		DisplayName:    agentID,
		RemoteURL:      remoteURL,
		AuthType:       authType,
		AuthConfigJSON: authConfigJSON,
		Enabled:        true,
		OrgID:          orgID,
		DiscoveredCard: a2abiz.AgentCard{Enabled: true, Capabilities: caps},
	})
}

// ---------- mock remote A2A endpoint ----------

type mockA2ARequest struct {
	Method string
	Text   string
	Header http.Header
}

// mockA2AEndpoint is an httptest server speaking the A2A JSON-RPC message/send
// method. It replies with a message whose text is "pong: <incoming text>".
type mockA2AEndpoint struct {
	srv        *httptest.Server
	mu         sync.Mutex
	requests   []mockA2ARequest
	requireKey string // when non-empty, X-Api-Key must match or 401 is returned
}

func newMockA2AEndpoint(t *testing.T) *mockA2AEndpoint {
	t.Helper()
	m := &mockA2AEndpoint{}
	m.srv = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockA2AEndpoint) url() string { return m.srv.URL }

func (m *mockA2AEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Method != "message/send" {
		writeJSONRPCError(w, req.ID, -32601, "method not found: "+req.Method)
		return
	}
	if m.requireKey != "" && r.Header.Get("X-Api-Key") != m.requireKey {
		http.Error(w, "missing or invalid api key", http.StatusUnauthorized)
		return
	}
	var params struct {
		Message struct {
			Parts []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	}
	_ = json.Unmarshal(req.Params, &params)
	text := ""
	if len(params.Message.Parts) > 0 {
		text = params.Message.Parts[0].Text
	}
	m.mu.Lock()
	m.requests = append(m.requests, mockA2ARequest{Method: req.Method, Text: text, Header: r.Header.Clone()})
	m.mu.Unlock()
	id := req.ID
	if len(id) == 0 {
		id = json.RawMessage(`"1"`)
	}
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"kind":      "message",
			"messageId": "srv-msg-1",
			"role":      "agent",
			"parts":     []map[string]any{{"kind": "text", "text": "pong: " + text}},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage(`"1"`)
	}
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mockA2AEndpoint) received() []mockA2ARequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockA2ARequest, len(m.requests))
	copy(out, m.requests)
	return out
}

// ---------- mTLS fixtures ----------

type testCA struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	pemDER []byte
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aranea-e2e-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}
	return testCA{cert: cert, key: key, pemDER: der}
}

// issue creates a cert signed by the CA. server=true → ServerAuth + IP SANs;
// server=false → ClientAuth.
func (ca testCA) issue(t *testing.T, commonName string, server bool, serial int64) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		tmpl.DNSNames = []string{"localhost"}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func (ca testCA) certPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

func (ca testCA) pemBytes() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.pemDER})
}

// newMockMTLSEndpoint starts an httptest TLS server that requires a client
// certificate signed by the test CA, speaking the same A2A handler.
func newMockMTLSEndpoint(t *testing.T, ca testCA) *mockA2AEndpoint {
	t.Helper()
	serverCertPEM, serverKeyPEM := ca.issue(t, "aranea-e2e-server", true, 2)
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("load server keypair: %v", err)
	}
	m := &mockA2AEndpoint{}
	m.srv = httptest.NewUnstartedServer(http.HandlerFunc(m.handle))
	m.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    ca.certPool(),
		MinVersion:   tls.VersionTLS12,
	}
	m.srv.StartTLS()
	t.Cleanup(m.srv.Close)
	return m
}

// writeClientAuthFiles writes the client cert/key and CA PEM to a temp dir and
// returns the mtls auth_config_json referencing them.
func writeClientAuthFiles(t *testing.T, ca testCA) string {
	t.Helper()
	dir := t.TempDir()
	clientCertPEM, clientKeyPEM := ca.issue(t, "aranea-e2e-client", false, 3)
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")
	caPath := filepath.Join(dir, "ca.crt")
	for path, data := range map[string][]byte{certPath: clientCertPEM, keyPath: clientKeyPEM, caPath: ca.pemBytes()} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	cfg, err := json.Marshal(map[string]string{
		"cert_file": certPath,
		"key_file":  keyPath,
		"ca_file":   caPath,
	})
	if err != nil {
		t.Fatalf("marshal auth config: %v", err)
	}
	return string(cfg)
}
