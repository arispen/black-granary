package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func doReq(t *testing.T, mux http.Handler, method, target string, form url.Values, pid string, remote string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if form != nil {
		bodyReader = strings.NewReader(form.Encode())
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, bodyReader)
	if remote == "" {
		remote = "127.0.0.1:12345"
	}
	req.RemoteAddr = remote
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if pid != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: pid})
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func cookieFromResponse(rr *httptest.ResponseRecorder, name string) string {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestHomeCookieAndDailyTickOncePerDay(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	r1 := doReq(t, mux, http.MethodGet, "/", nil, "", "127.0.0.1:1111")
	if r1.Code != http.StatusOK {
		t.Fatalf("GET / status=%d", r1.Code)
	}
	pid := cookieFromResponse(r1, cookieName)
	if pid == "" {
		t.Fatalf("expected pid cookie on first visit")
	}
	if s.TickCount != 1 {
		t.Fatalf("expected first daily tick to run once, got tickCount=%d", s.TickCount)
	}
	firstDate := s.LastDailyTickDate
	if firstDate == "" {
		t.Fatalf("expected LastDailyTickDate set")
	}

	r2 := doReq(t, mux, http.MethodGet, "/", nil, pid, "127.0.0.1:1111")
	if r2.Code != http.StatusOK {
		t.Fatalf("second GET / status=%d", r2.Code)
	}
	if s.TickCount != 1 {
		t.Fatalf("daily tick should not run again same day, got tickCount=%d", s.TickCount)
	}
}

func TestFragDashboardIncludesOOBUpdates(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	r := doReq(t, mux, http.MethodGet, "/frag/dashboard", nil, "", "127.0.0.1:1111")
	body := r.Body.String()
	if r.Code != http.StatusOK {
		t.Fatalf("GET /frag/dashboard status=%d", r.Code)
	}
	for _, want := range []string{
		`id="realm-header" hx-swap-oob="outerHTML"`,
		`id="event-log" hx-swap-oob="innerHTML"`,
		`id="players" hx-swap-oob="innerHTML"`,
		`id="diplomacy" hx-swap-oob="innerHTML"`,
		`id="institutions" hx-swap-oob="innerHTML"`,
		`id="intel" hx-swap-oob="innerHTML"`,
		`id="ledger" hx-swap-oob="innerHTML"`,
		`id="market" hx-swap-oob="innerHTML"`,
		`id="toast" hx-swap-oob="innerHTML"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard fragment missing %q", want)
		}
	}
}

func TestFragDashboardDoesNotConsumeToast(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)
	now := time.Now().UTC()

	s.mu.Lock()
	s.Players["p1"] = &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	setToastLocked(s, "p1", "Chat cooldown active.")
	s.mu.Unlock()

	body := doReq(t, mux, http.MethodGet, "/frag/dashboard", nil, "p1", "127.0.0.1:1111").Body.String()
	if !strings.Contains(body, "Chat cooldown active.") {
		t.Fatalf("dashboard poll should include current toast message")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if got := s.ToastByPlayer["p1"]; got != "Chat cooldown active." {
		t.Fatalf("dashboard poll should not consume toast, got %q", got)
	}
}

func TestDashboardStandingPanelAndStateBasedActions(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)
	now := time.Now().UTC()

	s.mu.Lock()
	s.Players["p1"] = &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Contracts["c1"] = &Contract{ID: "c1", Type: "Emergency", DeadlineTicks: 3, Status: "Issued"}
	s.mu.Unlock()

	body := doReq(t, mux, http.MethodGet, "/frag/dashboard", nil, "p1", "127.0.0.1:1111").Body.String()
	if !strings.Contains(body, "Your Standing in Black Granary") {
		t.Fatalf("standing panel should render on dashboard")
	}
	if !strings.Contains(body, "Institutions and Offices") {
		t.Fatalf("institutions panel should render on dashboard")
	}
	if !strings.Contains(body, ">Accept<") || !strings.Contains(body, ">Ignore<") {
		t.Fatalf("issued contract should show accept and ignore")
	}
	if !strings.Contains(body, `name="stance"`) {
		t.Fatalf("issued contract should include stance selector")
	}
	if strings.Contains(body, ">Abandon<") || strings.Contains(body, ">Deliver") {
		t.Fatalf("issued contract should not show abandon or deliver")
	}

	s.mu.Lock()
	s.Contracts["c1"].Status = "Accepted"
	s.Contracts["c1"].OwnerPlayerID = "p1"
	s.Contracts["c1"].OwnerName = "Ash Crow (Guest)"
	s.mu.Unlock()

	body = doReq(t, mux, http.MethodGet, "/frag/dashboard", nil, "p1", "127.0.0.1:1111").Body.String()
	if !strings.Contains(body, ">Abandon<") || !strings.Contains(body, "Deliver (&#43;20g)") {
		t.Fatalf("accepted contract should show deliver and abandon")
	}
	if !strings.Contains(body, "Stance: Careful") {
		t.Fatalf("accepted contract should show default stance")
	}
	if strings.Contains(body, ">Accept<") || strings.Contains(body, ">Ignore<") {
		t.Fatalf("accepted contract should not show accept or ignore")
	}
}

func TestFragEndpointsReturnInnerContentForPolling(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	events := doReq(t, mux, http.MethodGet, "/frag/events", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(events, `id="event-log"`) {
		t.Fatalf("/frag/events should return inner content only")
	}

	chat := doReq(t, mux, http.MethodGet, "/frag/chat", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(chat, `id="chat"`) {
		t.Fatalf("/frag/chat should return inner content only")
	}

	diplomacy := doReq(t, mux, http.MethodGet, "/frag/diplomacy", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(diplomacy, `id="diplomacy"`) {
		t.Fatalf("/frag/diplomacy should return inner content only")
	}

	players := doReq(t, mux, http.MethodGet, "/frag/players", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(players, `id="players"`) {
		t.Fatalf("/frag/players should return inner content only")
	}

	institutions := doReq(t, mux, http.MethodGet, "/frag/institutions", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(institutions, `id="institutions"`) {
		t.Fatalf("/frag/institutions should return inner content only")
	}

	intel := doReq(t, mux, http.MethodGet, "/frag/intel", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(intel, `id="intel"`) {
		t.Fatalf("/frag/intel should return inner content only")
	}

	ledger := doReq(t, mux, http.MethodGet, "/frag/ledger", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(ledger, `id="ledger"`) {
		t.Fatalf("/frag/ledger should return inner content only")
	}

	market := doReq(t, mux, http.MethodGet, "/frag/market", nil, "", "127.0.0.1:1111").Body.String()
	if strings.Contains(market, `id="market"`) {
		t.Fatalf("/frag/market should return inner content only")
	}
}

func TestAssetsAreServed(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	resp := doReq(t, mux, http.MethodGet, "/assets/icons/license.txt", nil, "", "127.0.0.1:1111")
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /assets/icons/license.txt status=%d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "https://game-icons.net") {
		t.Fatalf("asset payload should include expected license content")
	}
}

func TestActionAndChatRateLimits(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	home := doReq(t, mux, http.MethodGet, "/", nil, "", "127.0.0.1:1111")
	pid := cookieFromResponse(home, cookieName)
	if pid == "" {
		t.Fatalf("missing pid cookie")
	}

	a1 := doReq(t, mux, http.MethodPost, "/action", url.Values{"action": {"investigate"}}, pid, "127.0.0.1:1111")
	if a1.Code != http.StatusOK {
		t.Fatalf("first /action status=%d", a1.Code)
	}
	a2 := doReq(t, mux, http.MethodPost, "/action", url.Values{"action": {"investigate"}}, pid, "127.0.0.1:1111")
	if !strings.Contains(a2.Body.String(), "Slow down.") {
		t.Fatalf("expected action cooldown toast")
	}

	c1 := doReq(t, mux, http.MethodPost, "/chat", url.Values{"text": {"hello"}}, pid, "127.0.0.1:1111")
	if c1.Code != http.StatusOK {
		t.Fatalf("first /chat status=%d", c1.Code)
	}
	c2 := doReq(t, mux, http.MethodPost, "/chat", url.Values{"text": {"again"}}, pid, "127.0.0.1:1111")
	if !strings.Contains(c2.Body.String(), "Chat cooldown active.") {
		t.Fatalf("expected chat cooldown toast")
	}
}

func TestWhisperPrivacyOnFragChat(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)
	now := time.Now().UTC()

	s.mu.Lock()
	s.Players["p1"] = &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players["p2"] = &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players["p3"] = &Player{ID: "p3", Name: "Corin Reed (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	addChatLocked(s, ChatMessage{FromPlayerID: "p1", FromName: "Ash Crow (Guest)", Text: "global hello", Kind: "global", At: now})
	addChatLocked(s, ChatMessage{FromPlayerID: "p1", FromName: "Ash Crow (Guest)", ToPlayerID: "p2", ToName: "Bran Vale (Guest)", Text: "secret route", Kind: "whisper", At: now})
	s.mu.Unlock()

	bodyP2 := doReq(t, mux, http.MethodGet, "/frag/chat", nil, "p2", "127.0.0.1:1111").Body.String()
	if !strings.Contains(bodyP2, "secret route") {
		t.Fatalf("recipient should see whisper")
	}

	bodyP3 := doReq(t, mux, http.MethodGet, "/frag/chat", nil, "p3", "127.0.0.1:1111").Body.String()
	if strings.Contains(bodyP3, "secret route") {
		t.Fatalf("non-participant should not see whisper")
	}
	if !strings.Contains(bodyP3, "global hello") {
		t.Fatalf("all players should see global chat")
	}
}

func TestDiplomacyMessageDeliveryPrivacy(t *testing.T) {
	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)
	now := time.Now().UTC()

	s.mu.Lock()
	s.Players["p1"] = &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players["p2"] = &Player{ID: "p2", Name: "Bran Vale (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.Players["p3"] = &Player{ID: "p3", Name: "Corin Reed (Guest)", Gold: 20, Rep: 0, LastSeen: now}
	s.mu.Unlock()

	form := url.Values{
		"target_id": {"p2"},
		"subject":   {"Trade Offer"},
		"body":      {"Meet at dawn by the granary gate."},
	}
	resp := doReq(t, mux, http.MethodPost, "/message", form, "p1", "127.0.0.1:1111")
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /message status=%d", resp.Code)
	}

	bodyP2 := doReq(t, mux, http.MethodGet, "/frag/diplomacy", nil, "p2", "127.0.0.1:1111").Body.String()
	if !strings.Contains(bodyP2, "Meet at dawn by the granary gate.") {
		t.Fatalf("recipient should see message")
	}

	bodyP3 := doReq(t, mux, http.MethodGet, "/frag/diplomacy", nil, "p3", "127.0.0.1:1111").Body.String()
	if strings.Contains(bodyP3, "Meet at dawn by the granary gate.") {
		t.Fatalf("non recipient should not see message")
	}
}

func TestAdminProtectionTickAndReset(t *testing.T) {
	t.Setenv(adminTokenEnvName, "test-secret")
	t.Setenv(adminLoopbackEnvName, "false")
	t.Setenv(adminCookieSecureEnvName, "false")

	s := newTestStore()
	tmpl := parseTemplates()
	mux := newMux(s, tmpl)

	forbidden := doReq(t, mux, http.MethodGet, "/admin", nil, "", "203.0.113.10:9999")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected /admin forbidden without header/cookie auth, got %d", forbidden.Code)
	}

	queryToken := doReq(t, mux, http.MethodGet, "/admin?token=test-secret", nil, "", "203.0.113.10:9999")
	if queryToken.Code != http.StatusForbidden {
		t.Fatalf("query token should not authorize admin, got %d", queryToken.Code)
	}

	s.mu.Lock()
	s.Players["p1"] = &Player{ID: "p1", Name: "Ash Crow (Guest)", Gold: 20, Rep: 0, LastSeen: time.Now().UTC()}
	issueContractLocked(s, "Emergency", 3)
	prevTick := s.TickCount
	s.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.RemoteAddr = "203.0.113.10:9999"
	req.Header.Set(adminAuthHeaderName, "test-secret")
	okAdmin := httptest.NewRecorder()
	mux.ServeHTTP(okAdmin, req)
	if okAdmin.Code != http.StatusOK {
		t.Fatalf("expected /admin with header token to pass, got %d", okAdmin.Code)
	}
	adminCookie := cookieFromResponse(okAdmin, adminAuthCookieName)
	csrfCookie := cookieFromResponse(okAdmin, adminCSRFCookieName)
	if adminCookie == "" || csrfCookie == "" {
		t.Fatalf("expected admin and csrf cookies to be set")
	}

	noCSRFTick := httptest.NewRequest(http.MethodPost, "/admin/tick", strings.NewReader(""))
	noCSRFTick.RemoteAddr = "203.0.113.10:9999"
	noCSRFTick.AddCookie(&http.Cookie{Name: adminAuthCookieName, Value: adminCookie})
	noCSRFTick.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: csrfCookie})
	noCSRFTick.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	noCSRFTickResp := httptest.NewRecorder()
	mux.ServeHTTP(noCSRFTickResp, noCSRFTick)
	if noCSRFTickResp.Code != http.StatusForbidden {
		t.Fatalf("expected /admin/tick forbidden without csrf, got %d", noCSRFTickResp.Code)
	}

	tickReq := httptest.NewRequest(http.MethodPost, "/admin/tick", strings.NewReader(url.Values{"csrf_token": {csrfCookie}}.Encode()))
	tickReq.RemoteAddr = "203.0.113.10:9999"
	tickReq.AddCookie(&http.Cookie{Name: adminAuthCookieName, Value: adminCookie})
	tickReq.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: csrfCookie})
	tickReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tickResp := httptest.NewRecorder()
	mux.ServeHTTP(tickResp, tickReq)
	if tickResp.Code != http.StatusSeeOther {
		t.Fatalf("expected /admin/tick redirect with csrf, got %d", tickResp.Code)
	}
	s.mu.Lock()
	if s.TickCount != prevTick+1 {
		s.mu.Unlock()
		t.Fatalf("admin tick should increment TickCount")
	}
	s.mu.Unlock()

	resetReq := httptest.NewRequest(http.MethodPost, "/admin/reset", strings.NewReader(url.Values{"csrf_token": {csrfCookie}}.Encode()))
	resetReq.RemoteAddr = "203.0.113.10:9999"
	resetReq.AddCookie(&http.Cookie{Name: adminAuthCookieName, Value: adminCookie})
	resetReq.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: csrfCookie})
	resetReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resetResp := httptest.NewRecorder()
	mux.ServeHTTP(resetResp, resetReq)
	if resetResp.Code != http.StatusSeeOther {
		t.Fatalf("expected /admin/reset redirect with csrf, got %d", resetResp.Code)
	}
	s.mu.Lock()
	if len(s.Players) != 0 || len(s.Contracts) != 0 || s.TickCount != 0 {
		s.mu.Unlock()
		t.Fatalf("reset should clear world state")
	}
	s.mu.Unlock()
}
