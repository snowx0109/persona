package hub

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/nwverse/persona/server/internal/apierr"
	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/bus"
	"github.com/nwverse/persona/server/internal/compat"
	"github.com/nwverse/persona/server/internal/db"
	"github.com/nwverse/persona/server/internal/devicewire"
	"github.com/nwverse/persona/server/internal/extwire"
	"github.com/nwverse/persona/server/internal/i18n"
	"github.com/nwverse/persona/server/internal/identity"
	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/link"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
	"github.com/nwverse/persona/server/internal/sessionlist"
)

// nodeOutdated recognizes the prefix also returned by an old manager that lacks
// devicewire.MethodCall, even when that manager sets CodeUnknownMethod.
func (h *Hub) nodeOutdated(w http.ResponseWriter, r *http.Request, err error, key string, args i18n.Args) bool {
	var re *rpc.Error
	if key == "" || !errors.As(err, &re) || !strings.HasPrefix(re.Message, "unknown method") {
		return false
	}
	writeErrCode(routeLanguageWriter{ResponseWriter: w, language: h.requestLang(r)}, http.StatusConflict, proto.CodeNodeOutdated, i18n.Err(key, args))
	return true
}

func (h *Hub) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", h.health)
	h.passwordLoginRoutes(mux)
	h.browserLoginRoutes(mux)
	api := http.NewServeMux()
	h.apiMux = api
	if h.upd != nil {
		h.upd.routes(api)
	}
	h.machineRoutes(api)
	// Confirm / reject / rename a computer (machine_trust.go, machine_rename.go).
	h.machineTrustRoutes(api)
	h.machineRenameRoutes(api)
	h.deviceRoutesRelayed(api) // the manager's own: reconnect, engine, herdr, deploy, claude-dirs (devices_routes.go)
	h.sharingRoutes(api)
	h.fileRoutes(api)
	h.machineFileRoutes(api)
	h.gitRoutes(api)
	h.agentSettingsRoutes(api)
	h.sessionRoutes(api)
	h.paneRoutes(api)
	h.nodeDoctorRoutes(api)
	h.machineProcRoutes(api)
	h.nodeUpdateRelayRoutes(api)
	api.HandleFunc("GET /api/v1/interactions", func(w http.ResponseWriter, r *http.Request) {
		ixs := visibleInteractions(h.pendingInteractions(), h.requestHasElicitation(r))
		if restrictedAgentData(r) {
			em := extensionManager(r)
			ixs = slices.DeleteFunc(ixs, func(ix proto.Interaction) bool {
				return em != "" && cmp.Or(ix.Manager, assistantwire.AssistantID) != em || h.hiddenFromExtensions(ix)
			})
		}
		writeJSON(w, map[string]any{"interactions": h.withMuted(ixs)})
	})
	api.HandleFunc("GET /api/v1/ops/{id}", h.getClientOp)
	h.fleetRoutesRelayed(api) // resolve, history, seen: the manager's (fleet_link.go)
	h.phoneWriteRoutes(api)
	h.deviceRoutes(api)
	h.pushRoutes(api)
	h.webPushRoutes(api)
	h.live.routes(api)
	h.metricsRoutesRelayed(api) // the monitoring is the manager's (devices_routes.go)
	h.lockTestRoutes(api)
	h.assistantRoutes(api)
	h.pinRoutes(api)
	h.notifyRoutes(api)
	h.pushGradingRoutes(api)
	h.personaRoutes(api)
	h.lookRoutes(api)
	h.lookUploadRoutes(api)
	// usage windows (#2432): the manager's (devices_routes.go)
	h.usageRoutesRelayed(api)
	h.transferRoutesRelayed(api) // computer to computer file transfers, the manager's (devices_routes.go)
	h.inboxRoutesRelayed(api)
	// the extensions', connectors', bots', mail's, Cua's and ego's: the manager's (extensions_link.go)
	h.extensionRoutesRelayed(api)
	api.HandleFunc("GET /api/v1/ws", h.serveWS)
	h.tokenRoutes(api)
	h.portRoutes(api)
	h.previewRoutes(api)
	h.pairRoutes(api)
	h.managerRoutes(api) // the roster (managers.go)
	h.linkRoutes(api)
	h.publicRoutes(api)
	h.androidAppRoutes(api)
	h.peerRoutes(api)
	h.runtimeCLIRoutes(api)
	// What the center serves: its own apps' requests and those the members pass on (peer_http.go), whose source the
	// member judged (no gate here for them). A member passes everything but its own few routes on (memberForward).
	centerAPI := h.centerSharing(h.auth(h.quiesceGate(h.extensionAPIs(jsonRouteErrors(api), api))))
	h.mesh.centerAPI.Store(centerAPI)
	appAPI := h.gate(h.memberForward(centerAPI))
	h.busRoutes(mux)
	h.managerJoinRoutes(mux)
	h.managerInstallRoutes(mux) // GET /i/<code>: the install script (managers_install.go)
	mux.Handle("/api/", appAPI)
	mux.Handle("/", h.adminRoutes(mux, appAPI, webApp(filepath.Join(h.cfg.dir, "web"))))
	return h.outerLayers(h.sharingDispatcher(h.plaintextLoopbackOnly(jsonRouteErrors(mux))))
}

// outerLayers are what every request goes through around the routes, outermost first: the auth layer's marks
// (authMarks: outside gzip, which may hold a response's head back), gzip, the request log, a panic's recovery.
func (h *Hub) outerLayers(routes http.Handler) http.Handler {
	return authMarks(compatibilityRequests(gzipResponses(h.logf, logRequests(h, recoverPanics(h, routes)))))
}

// authMarks lets a response carry extwire.HeaderAuthRefused only when the hub's own auth layer refused the request
// (authRefused): off any other, a manager's or an extension's passed on included. Outermost, so the header is taken
// off as the head of the response actually leaves, not when a handler says its status: a writer in between (gzip)
// may hold the head back, and a header set meanwhile would go out with it.
func authMarks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mark := &authRefusal{}
		next.ServeHTTP(&markedWriter{ResponseWriter: w, mark: mark},
			r.WithContext(context.WithValue(r.Context(), authRefusalKey{}, mark)))
	})
}

// markedWriter is authMarks' writer: it takes extwire.HeaderAuthRefused off the head as it leaves, unless the request
// was marked.
type markedWriter struct {
	http.ResponseWriter
	mark *authRefusal
	sent bool // the final head is out
}

func (m *markedWriter) head(final bool) {
	if m.sent {
		return
	}
	if !m.mark.refused.Load() {
		m.ResponseWriter.Header().Del(extwire.HeaderAuthRefused) // a manager's or an extension's: never the hub's word
	}
	m.sent = final
}

func (m *markedWriter) WriteHeader(code int) {
	m.head(code >= 200) // (an informational answer goes, and the final head is still to come)
	m.ResponseWriter.WriteHeader(code)
}

func (m *markedWriter) Write(p []byte) (int, error) {
	m.head(true)
	return m.ResponseWriter.Write(p)
}

func (m *markedWriter) FlushError() error {
	m.head(true)
	return http.NewResponseController(m.ResponseWriter).Flush()
}

// Unwrap lets the handlers reach the underlying writer's Hijacker and deadlines.
func (m *markedWriter) Unwrap() http.ResponseWriter { return m.ResponseWriter }

func (h *Hub) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		public := h.sourceOf(r) == sourcePublic
		limited := public
		if _, fwd := forwardedOf(r); fwd {
			limited = false // the member that passed it on keeps the count (peer_http.go memberForward)
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		// An extension's token that fails counts in a bucket of its own, which gates extensions' tokens only: never the
		// bus, the apps or any other credential. A relayed manager's extensions call from its address, and calls the
		// hub can't place (the manager off the bus a moment) must not lock the manager itself out. There is nothing
		// to guess in one either: it is 32 random bytes, new each start.
		ext := strings.HasPrefix(tok, extTokenPrefix)
		failed := &h.devtok.failures
		if ext {
			failed = &h.devtok.extFailures
		}
		if limited {
			retry := h.devtok.failures.retry(authSourceKey(r), time.Now(), h.cfg.Security)
			if ext {
				retry = max(retry, failed.retry(authSourceKey(r), time.Now(), h.cfg.Security))
			}
			if retry > 0 {
				retryAfter(w, retry)
				h.securityError(w, r, http.StatusTooManyRequests, "rate_limited", 0)
				return
			}
		}
		// A manager's proof is read here and goes no further: not to a manager, a log or anything that copies headers.
		proof := r.Header.Get(extwire.HeaderManagerProof)
		if _, there := r.Header[http.CanonicalHeaderKey(extwire.HeaderManagerProof)]; there {
			r = r.Clone(r.Context())
			r.Header.Del(extwire.HeaderManagerProof)
		}
		// A proof that names a manager of the roster with no live bus connection here (the hub restarted, and the
		// manager has not noticed its connection is gone): nobody can tell whose the token is, nor check the proof
		// without that connection's key. A 503 says nothing of the token (32 random bytes: nothing to guess) and is
		// not counted, so the manager's extensions don't lock out its address until it is back.
		if ext && public && h.proofOfAManagerAway(r.Context(), proof) {
			retryAfter(w, 5*time.Second)
			authRefused(w, r, extwire.RefusedManagerOffline)
			h.securityError(w, r, http.StatusServiceUnavailable, codeManagerOffline, 0)
			return
		}
		var p principal
		var ok, sure bool
		if ext {
			p, ok, p.extOpens, p.extScope, sure = h.authorizeExtension(r.Context(), tok, r)
		} else {
			var err error
			if p, ok, err = h.lookupToken(r.Context(), tok); err != nil {
				// The token store did not answer: no verdict on the token. A 401 would tell the app its credential is
				// revoked (Android then forgets the hub), so say "try again" and don't count it against the source.
				retryAfter(w, 5*time.Second)
				h.securityError(w, r, http.StatusServiceUnavailable, codeTokenStoreUnavailable, 0)
				return
			}
		}
		if !ok && ext && !sure && (!public || h.proofVouches(r, proof, tok)) {
			// A manager did not answer (timed out, not ready yet): no verdict on the token, so neither counted nor
			// said to be nobody's (that would hold its manager's extensions). From the internet only for a request
			// its manager's proof vouches for: anything else is counted as ever.
			retryAfter(w, 5*time.Second)
			h.securityError(w, r, http.StatusServiceUnavailable, codeAuthorizeUnsure, 0)
			return
		}
		if !ok {
			failures := 0
			if limited {
				failures = failed.fail(authSourceKey(r), time.Now(), h.cfg.Security)
			}
			if ext && sure { // nobody's: a manager's relay holds its extensions' requests till its next bus connection
				authRefused(w, r, extwire.RefusedToken)
			}
			h.securityError(w, r, http.StatusUnauthorized, "", failures)
			return
		}
		if public && p.ID == "" {
			h.securityError(w, r, http.StatusForbidden, "master_token_local_only", 0)
			return
		}
		if p.Role == roleExtension {
			// (authenticate asked the manager whose it is and whether it opens this route: authorizeExtension)
			// An extension's token is for the hub it runs beside, on this machine (hub.url), or comes through its
			// manager's relay with the manager's proof: a relayed manager's computer may reach the hub only across the
			// internet (docs/assistant/managers.md "Extensions and the hub's API"; the gate still holds it to TLS there). A
			// token alone from there is refused, and counted against the source; a proof the manager did sign but
			// that is out of time, sent twice or one too many just now is refused without counting.
			if public {
				switch h.extensionProof(r, proof, tok, p.Manager) {
				case proofOK:
				case proofSkew:
					h.securityError(w, r, http.StatusForbidden, codeClockSkew, 0)
					return
				case proofReplayed:
					h.securityError(w, r, http.StatusForbidden, codeProofReplayed, 0)
					return
				case proofBusy:
					retryAfter(w, extwire.ProofWindow)
					h.securityError(w, r, http.StatusTooManyRequests, codeProofBusy, 0)
					return
				default:
					failures := 0
					if limited {
						failures = failed.fail(authSourceKey(r), time.Now(), h.cfg.Security)
					}
					h.securityError(w, r, http.StatusForbidden, "extension_token_local_only", failures)
					return
				}
			}
			if ok, scope := p.extOpens, p.extScope; !ok {
				if scope == extwire.ScopeAgentsPermissionMode { // audited like every refused mode (extension_modes.go)
					h.refuseModeScope(w, r, p)
					return
				}
				body := errorBody(http.StatusForbidden, "permission_denied", errors.New("this extension was not granted this part of the hub's API"))
				if scope != "" {
					body["scope"] = scope
				}
				writeErrorBody(w, http.StatusForbidden, body)
				return
			}
			// Its manager's machines, sessions and cards only (ownership.go extensionReach).
			if err := h.extensionReach(r, p); err != nil {
				writeErrCode(w, http.StatusForbidden, codeOtherManager, err)
				return
			}
		} else if p.Role == roleScoped {
			if !h.authorizeScopedDevice(w, r, p) {
				return
			}
		} else if !allowed(p, r.Method, r.URL.Path) {
			msg := "this device can only read and answer"
			if p.Role == roleLink || p.Role == roleLinkPair {
				msg = "a Persona Link credential only opens its machine's link"
			} else if p.Role == rolePeer || p.Role == rolePeerPair {
				msg = "a peer credential only opens the mesh link"
			}
			writeErr(w, http.StatusForbidden, errors.New(msg))
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
		if p.PairHash != "" && r.URL.Path == "/api/v1/ws" {
			ctx, cancel := h.pairContext(r.Context(), p)
			defer cancel()
			r = r.WithContext(ctx)
		}
		// API answers are live data: without this the phone's URLCache kept a copy of every page it pulled (megabytes,
		// never read again). Images and media that may be cached set their own Cache-Control.
		w.Header().Set("Cache-Control", "no-store")
		h.observeClientRequest(next, w, r, p)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status          int
	securityFailure bool
	lang            i18n.Lang
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// authRefusal is a request's mark that the hub's own auth layer refused it (authMarks puts it in the context).
type authRefusal struct{ refused atomic.Bool }

type authRefusalKey struct{}

// authRefused says on the response that the hub's auth layer refuses the request (extwire.HeaderAuthRefused, what:
// RefusedToken or RefusedManagerOffline). Set by nothing else: the response writer takes the header off any other.
func authRefused(w http.ResponseWriter, r *http.Request, what string) {
	if m, ok := r.Context().Value(authRefusalKey{}).(*authRefusal); ok {
		m.refused.Store(true)
	}
	w.Header().Set(extwire.HeaderAuthRefused, what)
}

// Lang is the request's language: apierr writes an i18n.Message error in it.
func (s *statusWriter) Lang() i18n.Lang { return s.lang }

// Unwrap lets websocket.Accept reach the underlying Hijacker.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logRequests logs failed and slow requests (method, path, status, time; never the query, body or token) under
// a request id: the app's X-Client-Request-Id when it sent one, which it shows next to a server error, else a new
// one. Every response carries it as X-Request-Id.
func logRequests(h *Hub, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		client := requestClient{}
		if !compat.HeaderConflict(r.Header, "X-Persona-Client") {
			client = parseRequestClient(compat.Header(r.Header, "X-Persona-Client"))
		}
		r = r.WithContext(context.WithValue(r.Context(), requestClientKey{}, client))
		id := requestID(r.Header.Get("X-Client-Request-Id"))
		w.Header().Set("X-Request-Id", id)
		// the language of this response's text for people: its Accept-Language, else the user's (locale.go)
		lang := i18n.FromRequest(r).Or(h.userLang())
		r = r.WithContext(i18n.With(r.Context(), lang))
		sw := &statusWriter{ResponseWriter: w, status: 200, lang: lang}
		next.ServeHTTP(sw, r)
		// Auth and ingress refusals are logged by securityError (masked, rate limited), WebSocket ones included.
		if !sw.securityFailure && (sw.status >= 400 || (!longLived(r) && time.Since(start) > 2*time.Second)) {
			path := loggedPath(r.URL.Path)
			if client.Platform != "" {
				h.logf("[%s] %s %s -> %d (%s) client=%q", id, r.Method, path, sw.status, time.Since(start).Round(time.Millisecond), client.Platform+"/"+client.Version)
			} else {
				h.logf("[%s] %s %s -> %d (%s)", id, r.Method, path, sw.status, time.Since(start).Round(time.Millisecond))
			}
		}
	})
}

// loggedPath is a request's path as the log may show it: an install code in /i/<code> (a credential) is masked.
func loggedPath(p string) string {
	for _, prefix := range []string{"/share/", publicSharePrefix} {
		if strings.HasPrefix(p, prefix) {
			return prefix + "<capability>"
		}
	}
	if rest, ok := strings.CutPrefix(p, installScriptPath); ok {
		code, tail, _ := strings.Cut(rest, "/")
		if code != "" {
			if tail != "" {
				tail = "/" + tail
			}
			return installScriptPath + "<code>" + tail
		}
	}
	return p
}

// longLived requests (WebSockets, a plugin's sockets and streams, a meeting's audio socket, a forwarded port's
// connections: ports.go) are not slow requests however long they last.
func longLived(r *http.Request) bool {
	path := r.URL.Path
	return path == "/api/v1/ws" || path == link.Path || path == link.StreamPath || path == bus.Path ||
		strings.HasPrefix(path, "/api/v1/plugins/") || strings.HasPrefix(path, "/api/v1/extensions/") || strings.HasSuffix(path, "/connect") ||
		isAssistantSocket(r)
}

// requestID keeps a client's id if it is a short token (it lands in logs), else makes one.
func requestID(client string) string {
	if n := len(client); n >= 4 && n <= 32 && strings.Trim(client, "0123456789abcdefABCDEF-") == "" {
		return client
	}
	return randomHex(4)
}

func writeJSON(w http.ResponseWriter, v any) { apierr.WriteJSON(w, v) }

// marshalItems is v, a page or push of transcript items, as JSON with < > & as they are. Escaped, each costs six
// bytes, and a tool input or text of HTML or code went to the apps at up to three times its size (#2907). Escapes the
// node already wrote stay as they are, and U+2028 and U+2029 are still escaped, as encoding/json does without HTML
// escaping.
func marshalItems(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := itemsEncoder(&b).Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// writeItemsJSON is writeJSON for a body of transcript items (marshalItems).
func writeItemsJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = itemsEncoder(w).Encode(v)
}

func itemsEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// agentTypeRe is what POST /api/v1/sessions takes as agent_type: claude, codex, traex, or a runtime's agent as the
// launcher list says (a runtime's launcher starts through its runtime, whatever the type); the node checks the rest.
var agentTypeRe = lazyre.New(`^[a-z][a-z0-9_-]{0,31}$`)

func (h *Hub) withMachine(fn func(http.ResponseWriter, *http.Request, *Machine)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := h.localMachine(r.PathValue("mid"))
		if m == nil {
			writeErr(w, http.StatusNotFound, i18n.Err("devices-api.unknown_machine"))
			return
		}
		if m.conversationDeleted(r.PathValue("sid")) {
			writeErrCode(w, http.StatusGone, "conversation_deleted", i18n.Err("assistant-conversation_delete.deleted"))
			return
		}
		if sid := r.PathValue("sid"); sid != "" {
			w = &conversationResponseWriter{ResponseWriter: w, machine: m, id: sid, header: w.Header().Clone()}
		}
		fn(w, r, m)
	}
}

func callErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errManagerOffline) { // the manager that carries the machine is not there (devices_link.go)
		writeErrCode(w, http.StatusServiceUnavailable, assistantwire.CodeAssistantOffline, errAssistantOffline)
		return
	}
	apierr.Call(w, err)
}

// callNode relays a raw JSON result and keeps each caller's timeout and error mapping.
func (h *Hub) callNode(w http.ResponseWriter, r *http.Request, m *Machine, method string, params any, timeout time.Duration, outdated string, args i18n.Args, handle ...func(error) bool) (json.RawMessage, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	if strings.HasPrefix(method, "git.") {
		var err error
		method, params, err = backendGitTarget(r, method, params)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return nil, false
		}
	}
	var out json.RawMessage
	if err := m.call(ctx, method, params, &out); err != nil {
		if h.nodeOutdated(w, r, err, outdated, args) {
			return nil, false
		}
		for _, f := range handle {
			if f(err) {
				return nil, false
			}
		}
		callErr(w, err)
		return nil, false
	}
	return out, true
}

func writeRawJSON(w http.ResponseWriter, out json.RawMessage) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(out)
}

// proxy forwards a GET to the machine's node and returns its result verbatim.
func (h *Hub) proxy(method string, params func(*http.Request) any) http.HandlerFunc {
	return h.withMachine(func(w http.ResponseWriter, r *http.Request, m *Machine) {
		var p any = map[string]any{}
		if params != nil {
			p = params(r)
		}
		out, ok := h.callNode(w, r, m, method, p, 30*time.Second, "", nil)
		if !ok {
			return
		}
		writeRawJSON(w, out)
	})
}

// sessionItems pages through a session's history. With lazy_images=1 images come without their data (the app
// fetches each one from sessionImage when it scrolls into view); older apps still get them inline.
func (h *Hub) sessionItems(w http.ResponseWriter, r *http.Request, m *Machine) {
	q := r.URL.Query()
	params := map[string]any{"session_id": r.PathValue("sid")}
	strict := q.Get("strict") == "1"
	if strict {
		params["strict_read"] = true
	}
	userInputs := q.Get("user_inputs") == "1"
	if userInputs {
		params["user_inputs"] = true // only a count/position projection, never message bodies
	}
	// Only the paging the app gave: one it left out or sent empty (iOS's latest page has before_seq=) is not 0 to a
	// runtime, which may take before_seq 0 as a bad value and refuse every page, and the app's sync then asks again.
	for _, k := range []string{"before_seq", "limit", "turns"} {
		if v := q.Get(k); v != "" {
			params[k] = atoi(v)
		}
	}
	if q.Has("after_seq") { // what came after the app's newest item (background sync); has_more is meaningless here
		params["after_seq"] = atoi(q.Get("after_seq"))
	}
	if q.Get("lazy_images") == "1" {
		params["lazy_images"] = true // the node keeps images out of its items; it would read them back for nothing
	}
	if q.Get("fold_work") == "1" {
		params["fold_work"] = true // runs of tools and thoughts come as one fold each (transcript.FoldWork)
	}
	if q.Has("through_seq") { // opening a fold: its lines, from_seq through through_seq
		params["from_seq"], params["through_seq"] = atoi(q.Get("from_seq")), atoi(q.Get("through_seq"))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if userInputs {
		var index proto.UserInputIndex
		if err := m.call(ctx, "session.items", params, &index); err != nil {
			callErr(w, err)
			return
		}
		if index.Inputs == nil || index.Total != len(index.Inputs) {
			writeErr(w, http.StatusBadGateway, errors.New("incomplete user input index"))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, index)
		return
	}
	var res struct {
		Items      []proto.Item `json:"items"`
		HasMore    bool         `json:"has_more"`
		MoreAfter  bool         `json:"more_after,omitempty"`
		More       bool         `json:"more,omitempty"` // a fold's lines go on after these
		StrictRead bool         `json:"strict_read,omitempty"`
		Partial    bool         `json:"partial,omitempty"`
		itemSpan
	}
	if err := m.call(ctx, "session.items", params, &res); err != nil {
		callErr(w, err)
		return
	}
	if strict && !res.StrictRead {
		writeErrCode(w, http.StatusConflict, proto.CodeNodeOutdated, i18n.Err("chatexport.node_version"))
		return
	}
	res.Items = cardCompatItems(h.requestLang(r), h.requestWants(r, featureCardRing), h.requestWants(r, featureMarkdownCloseout), localizedItems(h.requestLang(r), h.withInjected(res.Items)))
	if q.Get("lazy_images") == "1" {
		res.Items = proto.WithoutImageData(res.Items)
	}
	writeItemsJSON(w, res)
}

// sessionItemFull serves one item whole: pages cut long text (the item says full_size), the app fetches the rest when
// the user opens it.
func (h *Hub) sessionItemFull(w http.ResponseWriter, r *http.Request, m *Machine) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	params := map[string]any{"session_id": r.PathValue("sid"), "item_id": r.PathValue("iid")}
	strict := r.URL.Query().Get("strict") == "1"
	if strict {
		params["strict_read"] = true
	}
	var full struct {
		proto.Item
		StrictRead      bool `json:"strict_read,omitempty"`
		SourceTextBytes *int `json:"source_text_bytes,omitempty"`
	}
	if err := m.call(ctx, "session.item", params, &full); err != nil {
		callErr(w, err)
		return
	}
	if strict && !full.StrictRead {
		writeErrCode(w, http.StatusConflict, proto.CodeNodeOutdated, i18n.Err("chatexport.node_version"))
		return
	}
	it := full.Item
	if !strict {
		// A client opened an item whole. Let the channels following its view see it too (fleet_follow.go).
		h.asstOf(h.sessionOwner(m.cfg.ID, r.PathValue("sid"))).notify(assistantwire.EventSessionItems, assistantwire.SessionItems{MachineID: m.cfg.ID, SessionID: r.PathValue("sid"),
			Items: []proto.Item{it}})
	}
	full.Item = cardCompatItems(h.requestLang(r), h.requestWants(r, featureCardRing), h.requestWants(r, featureMarkdownCloseout), localizedItems(h.requestLang(r), h.withInjected([]proto.Item{it})))[0]
	writeItemsJSON(w, full)
}

// sessionImage serves one item's image as raw bytes. An item's image never changes, so the app may cache it forever.
func (h *Hub) sessionImage(w http.ResponseWriter, r *http.Request, m *Machine) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var img proto.Image
	if err := m.call(ctx, "session.image", map[string]any{"session_id": r.PathValue("sid"), "item_id": r.PathValue("iid")}, &img); err != nil {
		callErr(w, err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if img.Mime != "" {
		w.Header().Set("Content-Type", img.Mime)
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// sessionVisualization serves one visualization item's HTML fragment as raw UTF-8. A fragment never changes, so the
// app may cache it forever.
func (h *Hub) sessionVisualization(w http.ResponseWriter, r *http.Request, m *Machine) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var html string
	if err := m.call(ctx, "session.visualization", map[string]any{"session_id": r.PathValue("sid"), "item_id": r.PathValue("iid")}, &html); err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write([]byte(html))
}

// proxyBody forwards a POST body (plus session_id from the path) to the node.
func (h *Hub) proxyBody(method string) http.HandlerFunc {
	// Resuming an offline session can take a while (shell + claude startup).
	return h.proxyBodyFor(method, 90*time.Second)
}

// proxyBodyFor is proxyBody that waits up to timeout for the node.
func (h *Hub) proxyBodyFor(method string, timeout time.Duration, mutate ...func(*http.Request, map[string]any)) http.HandlerFunc {
	return h.withMachine(func(w http.ResponseWriter, r *http.Request, m *Machine) {
		p, ok := optionalObject(w, r, 32<<20)
		if !ok {
			return
		}
		p["session_id"] = r.PathValue("sid")
		for _, f := range mutate {
			f(r, p)
		}
		out, ok := h.callNode(w, r, m, method, p, timeout, "", nil)
		if !ok {
			return
		}
		writeRawJSON(w, out)
	})
}

// interruptSession stops a session's turn (session.interrupt). The manager retires any pending follow-up for every
// known session; a reporting Assistant conversation is also marked stopped for its plugin (#3308).
func (h *Hub) interruptSession(w http.ResponseWriter, r *http.Request, m *Machine) {
	p, ok := optionalObject(w, r, 32<<20)
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	p["session_id"] = sid
	r = h.captureHumanSubmission(r, "")
	out, ok := h.callNode(w, r, m, "session.interrupt", p, 90*time.Second, "", nil)
	if !ok {
		return
	}
	if _, ok := m.session(sid); ok {
		h.asstOf(h.sessionOwner(m.cfg.ID, sid)).UserStopped(r.Context(), assistantwire.SessionRef{MachineID: m.cfg.ID, SessionID: sid})
	}
	writeRawJSON(w, out)
}

func (h *Hub) listSessions(w http.ResponseWriter, r *http.Request) {
	named, ok := h.sessionsManager(w, r) // ?manager=<id>: that manager's computers only (sessions_list.go)
	if !ok {
		return
	}
	// include_inactive=1: sessions active in a time window, inactive ones too (sessions_history.go). Without it, as
	// before: since / cursor / limit stay the indexed list's.
	if history, err := historyMode(r); err != nil {
		sessionListErr(w, err)
		return
	} else if history {
		h.historySessions(w, r, named)
		return
	}
	_, lookup := r.URL.Query()["session_id"]
	if lookup || sessionlist.Enabled(r.URL.Query()) {
		h.pagedSessions(w, r, named)
		return
	}
	mid := r.URL.Query().Get("machine_id")
	em := extensionManager(r) // an extension's token: its manager's machines only
	if em != "" && mid != "" && h.machineOwner(mid) != em {
		writeErrCode(w, http.StatusForbidden, codeOtherManager, errOtherManager)
		return
	}
	// Each machine's list as kept, written out a session at a time with its manager (decorated): with thousands of
	// sessions, a copy of all the lists and the whole answer in one buffer were most of what an answer allocated.
	var lists []stampedList
	for _, m := range h.machinesNow() {
		owner := h.machineOwner(m.cfg.ID)
		if mid != "" && m.cfg.ID != mid || em != "" && owner != em || named != "" && owner != named {
			continue
		}
		lists = append(lists, stampedList{withoutHealthSessions(r, h.shown(m.cfg.ID, m.keptSessions())), owner})
	}
	writeSessionLists(w, lists)
}

// stampedList is a machine's list as kept, and the manager its sessions go out with.
type stampedList struct {
	list    []proto.Session
	manager string
}

// sessionsFlushAt is how much of the answer writeSessionLists holds before writing it out; its buffer has room for
// one more session of up to 8 KB on top.
const sessionsFlushAt = 32 << 10

// writeSessionLists writes {"sessions":[...]} of every session of lists, each stamped with its list's manager: the
// bytes writeJSON gives for the stamped lists put together, encoded a session at a time and written out every
// sessionsFlushAt bytes, so neither a copy of the lists nor the whole answer is held. A session that fails to encode
// is left out, as a push leaves it out (encodeRows). The lists are only read.
//
// Each row goes out bounded (sessions_list.go legacyListCut): long texts cut and the task arrays left out — a fleet
// of hundreds of agents made the whole-list answer hundreds of KB a pull, and a row's whole detail stays in the chat
// routes. The scratch copy keeps the kept list unchanged.
func writeSessionLists(w http.ResponseWriter, lists []stampedList) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	var buf bytes.Buffer
	buf.Grow(sessionsFlushAt + 8<<10)
	buf.WriteString(`{"sessions":[`)
	enc := json.NewEncoder(&buf) // escapes HTML, as writeJSON does
	var scratch proto.Session
	n := 0
	for _, l := range lists {
		for i := range l.list {
			scratch = l.list[i] // a plain struct copy: the slices point into the kept list, only nilled here
			scratch.Manager = l.manager
			legacyListCut(&scratch)
			mark := buf.Len()
			if n > 0 {
				buf.WriteByte(',')
			}
			if enc.Encode(&scratch) != nil {
				buf.Truncate(mark)
				continue
			}
			buf.Truncate(buf.Len() - 1) // the '\n' Encode ends each value with
			n++
			if buf.Len() >= sessionsFlushAt {
				if _, err := w.Write(buf.Bytes()); err != nil {
					return // the client is gone
				}
				buf.Reset()
			}
		}
	}
	buf.WriteString("]}\n")
	_, _ = w.Write(buf.Bytes())
}

func (h *Hub) startSession(w http.ResponseWriter, r *http.Request) {
	var p struct {
		MachineID       string        `json:"machine_id"`
		Cwd             string        `json:"cwd"`
		Launcher        string        `json:"launcher"`
		AgentType       string        `json:"agent_type,omitempty"`
		WorkSession     bool          `json:"work_session,omitempty"`
		Model           string        `json:"model,omitempty"` // picked in "New agent" for this start; empty: applyModel's
		Prompt          string        `json:"prompt"`
		Images          []proto.Image `json:"images,omitempty"`
		ClientMsgID     string        `json:"client_msg_id,omitempty"`
		ResumeSessionID string        `json:"resume_session_id"`
		WorkspaceID     string        `json:"workspace_id"`
		// Host: the machine's enabled engine host to open in (herdr, orca, paseo…; #2576); empty: its default host.
		// Checked by the node (an older node ignores it and starts in its engine).
		Host           string `json:"host,omitempty"`
		PermissionMode string `json:"permission_mode,omitempty"`
		Effort         string `json:"effort,omitempty"`
		// LauncherDefaultEffort keeps an explicit launcher's effort at its native default.
		LauncherDefaultEffort bool   `json:"launcher_default_effort,omitempty"`
		Speed                 string `json:"speed,omitempty"`
		Ultracode             *bool  `json:"ultracode,omitempty"`
		// Goal: a one-line condition the node sets with /goal after the first message is in (node/startgoal.go); a
		// start it cannot take there answers 409 start_goal_unsupported before anything starts.
		Goal string `json:"goal,omitempty"`
		// TabName names the herdr tab; empty: one made from the first message (tabName, assistant/agents_start.go),
		// else agent-<n>.
		TabName string `json:"tab_name,omitempty"`
		// Async: answer at once with the session it is going to be and start it in the background (node/starting.go);
		// messages sent to it meanwhile go in once it is ready. NameHint: what the tab name is made from when the
		// app sends the first message itself (so it shows as a message right away) instead of as prompt.
		Async    bool   `json:"async,omitempty"`
		NameHint string `json:"name_hint,omitempty"`
		TodoID   string `json:"todo_id,omitempty"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err == nil {
		err = json.Unmarshal(body, &p)
	}
	if err != nil {
		writeBodyErr(w, err, err)
		return
	}
	if p.WorkSession && (p.Launcher == "" || p.AgentType == "" || strings.TrimSpace(p.Cwd) == "" ||
		p.ResumeSessionID != "" || p.TodoID != "" || p.WorkspaceID != "" || p.Host != "") {
		writeErrCode(w, http.StatusBadRequest, "work_target_required", errors.New("Choose a computer, project folder, harness and launcher for the new session."))
		return
	}
	r = h.captureHumanSubmission(r, p.Prompt)
	if p.TodoID != "" && principalOf(r.Context()).Role != roleFull {
		writeErr(w, http.StatusForbidden, errors.New("read-only role cannot modify todos"))
		return
	}
	// An extension's token starts (or resumes) agents on its own manager's machines only (ownership.go).
	if _, err := h.extensionsOwn(r, h.sessionOwner(p.MachineID, p.ResumeSessionID)); err != nil {
		writeErrCode(w, http.StatusForbidden, codeOtherManager, err)
		return
	}
	// Resuming with a permission mode changes that session's mode: an extension's token, only of a conversation its
	// extension opened (extension_modes.go).
	pr := principalOf(r.Context())
	if pr.Role == roleScoped && (p.PermissionMode != "" || p.ResumeSessionID != "") && !slices.Contains(pr.Scopes, extwire.ScopeAgentsPermissionMode) {
		scopedPermissionDenied(w, extwire.ScopeAgentsPermissionMode)
		return
	}
	m := h.localMachine(p.MachineID)
	if m == nil {
		writeErr(w, http.StatusNotFound, i18n.Err("devices-api.unknown_machine"))
		return
	}
	if pr.Role == roleExtension && p.ResumeSessionID != "" && p.PermissionMode != "" {
		if why := h.extensionModeRefused(pr, p.MachineID, p.ResumeSessionID, map[string]any{"permission_mode": p.PermissionMode}); why != "" {
			h.refuseExtensionMode(w, r, pr, p.MachineID, p.ResumeSessionID, p.PermissionMode, why)
			return
		}
	}
	if restrictedAgentData(r) {
		// A scoped token resumes only a chat its computer lists, shown and outside the health space. An extension's
		// resume is decided last (extensionResume): one its computer no longer lists goes to the computer's check (#10941).
		if pr.Role == roleScoped && p.ResumeSessionID != "" && !h.publicSessionForControl(m, p.ResumeSessionID) {
			writeErr(w, http.StatusNotFound, errDeleteUnknown)
			return
		}
		if identity.InHealthSpace(startCwd(m, p.Cwd)) {
			writeErrCode(w, http.StatusForbidden, "permission_denied", i18n.Err("apierr-apierr.health_start_forbidden"))
			return
		}
	}
	if h.machineUntrusted(m.cfg.ID) { // not confirmed yet (machine_trust.go)
		writeErrCode(w, http.StatusConflict, "machine_pending", i18n.Err("hub-machine_trust.start_pending"))
		return
	}
	decided := ""
	var handoffImages []proto.Image
	if p.TodoID != "" {
		// The manager checks the todo (404, 409, 503 assistant_offline while it is away) and puts its decisions before
		// the prompt, a new agent's project background before those (#1618, #2820); they go to the agent, not into
		// its tab's name (nameFrom is taken from the prompt as sent).
		hand := assistantwire.TodoHandoff{TodoID: p.TodoID, Text: p.Prompt}
		if p.ResumeSessionID == "" {
			hand.New, hand.MachineID, hand.Cwd = true, m.cfg.ID, startCwd(m, p.Cwd)
		}
		var ok bool
		if decided, handoffImages, ok = h.todoHandoff(r.Context(), w, hand); !ok {
			return
		}
	}
	if p.AgentType == "" {
		for _, launcher := range m.cfg.Launchers {
			if launcher.Name == p.Launcher {
				p.AgentType = launcher.AgentType
				break
			}
		}
	}
	if p.AgentType != "" && !agentTypeRe.MatchString(p.AgentType) {
		writeErr(w, http.StatusBadRequest, errors.New("unsupported agent type"))
		return
	}
	p.TabName = strings.TrimSpace(p.TabName)
	nameFrom := p.Prompt
	if strings.TrimSpace(p.NameHint) != "" {
		nameFrom = p.NameHint
	}
	if p.TodoID != "" {
		p.Prompt = decided
	}
	if strings.TrimSpace(p.Prompt) != "" {
		p.Images = append(p.Images, handoffImages...)
	}
	async := startsAsync(p.Async, p.ResumeSessionID, p.Launcher, p.AgentType)
	nameAfterStart := async
	parent := r.Context()
	if r.Header.Get("X-Persona-Op-Id") != "" {
		// Finish an accepted start even if its HTTP response is lost; client_ops records the outcome.
		parent = context.WithoutCancel(parent)
	}
	startedAt := time.Now()
	budget := startBudget(async && !p.WorkSession)
	ctx, cancel := context.WithDeadline(parent, startedAt.Add(budget))
	defer cancel()
	var res struct {
		Session      proto.Session `json:"session"`
		LauncherNote string        `json:"launcher_note,omitempty"` // a default it lacks gave way to another (#4661)
		// Instead: what an engine that runs only its own launchers ran in place of what was asked (#8140, the node's
		// answer passed on); launcher_note says it.
		Instead *proto.StartInstead `json:"instead,omitempty"`
	}
	raw, _ := json.Marshal(p)
	params := map[string]any{}
	_ = json.Unmarshal(raw, &params)
	name := "" // a tab named now, before it opens; one started in the background is renamed when its name comes (below)
	if !async && p.TabName == "" && p.ResumeSessionID == "" {
		name = nameFrom
	}
	// The manager says what it runs on (Models › Agents, a model picked for this start) and names its tab
	// (agents_link.go).
	note, err := h.agentStartParams(ctx, m, params, name)
	if err != nil {
		var refused *startRefused
		if errors.As(err, &refused) {
			writeErr(w, 400, err)
			return
		}
		callErr(w, err) // 503 assistant_offline without the manager (the machine is its to reach), else 502
		return
	}
	resolvedLauncher, _ := params["launcher"].(string)
	resolvedAgent, _ := params["agent_type"].(string)
	async = startsAsync(p.Async, p.ResumeSessionID, resolvedLauncher, resolvedAgent)
	if resolvedBudget := startBudget(async && !p.WorkSession); resolvedBudget != budget {
		// The manager may have found a synchronous launcher after the request was admitted.
		cancel()
		resolvedCtx, cancelResolved := context.WithDeadline(parent, startedAt.Add(resolvedBudget))
		defer cancelResolved()
		ctx = resolvedCtx
	}
	method := "session.start"
	if p.WorkSession {
		method = devicewire.MethodStartWorkSession // An old node must refuse, never silently ignore strict start options.
		if params["launcher"] != p.Launcher || (strings.TrimSpace(p.Model) != "" && params["model"] != strings.TrimSpace(p.Model)) {
			writeErrCode(w, http.StatusConflict, "work_target_changed", errors.New("The selected launcher or model changed."))
			return
		}
		if strings.TrimSpace(p.Model) == "" {
			// In this picker "Launcher default" is the launcher's own model,
			// even if Models › Agents happens to configure the same launcher.
			delete(params, "model")
			delete(params, "model_fingerprint")
		}
		if p.Effort == "" {
			delete(params, "effort")
		}
		setWorkLauncherConfig(m, p.Launcher, params)
	}
	// An extension's token resumes a conversation its computer lists outside the health space, or one it does not
	// list that the computer checks (extensionResume, #10941). Decided last, once every check that holds for any id
	// has passed: a request one of them refuses is answered the same whichever conversation it names.
	checkedResume := false
	if pr.Role == roleExtension && p.ResumeSessionID != "" {
		var ok bool
		if checkedResume, ok = h.extensionResume(w, m, p.ResumeSessionID); !ok {
			return
		}
	}
	if checkedResume {
		params["resume_outside_health"] = true // nodeproto.ResumeOutsideHealth: set after the manager's fill, never the caller's
	}
	if err := m.call(ctx, method, params, &res); err != nil {
		h.startEnded(m.cfg.ID, p.ResumeSessionID, params, err)
		if re := (*rpc.Error)(nil); checkedResume && errors.As(err, &re) && re.Code == proto.CodeResumeUnknown {
			// The answer the hub gives a conversation it lists in the space or does not know: the extension is not
			// told which it was.
			writeErr(w, http.StatusNotFound, errDeleteUnknown)
			return
		}
		callErr(w, apierr.OnMachine(err, m.name())) // a launcher the new tab cannot run "on <name>'s MacBook"
		return
	}
	// An engine that runs only its own launchers may have started one of the same kind in place of the launcher asked
	// for (#8140): its health and a model's use go to the launcher that ran, not to the one that did not.
	if res.Instead != nil && res.Instead.Launcher != "" && res.Session.Launcher != "" {
		params["launcher"] = res.Session.Launcher
	}
	if !async {
		h.startEnded(m.cfg.ID, p.ResumeSessionID, params, nil)
	}
	if model, _ := params["model"].(string); p.Model != "" && p.ResumeSessionID == "" && model == strings.TrimSpace(p.Model) &&
		(res.Instead == nil || res.Instead.Model == "") { // a model the launcher that ran does not offer did not run
		launcher, _ := params["launcher"].(string) // the one it ran with: the manager filled it in
		if async {
			// Acknowledged before it ran: the launcher's check and the agent coming up are still to come.
			go h.modelUsedOnceStarted(m.cfg.ID, res.Session.SessionID, launcher, model)
		} else {
			h.modelUsed(h.machineOwner(m.cfg.ID), m.cfg.ID, launcher, model)
		}
	}
	res.Session.MachineID = m.cfg.ID
	if shown, ok := h.awaitListed(ctx, m, res.Session.SessionID); ok {
		res.Session.State = shown.State // as the manager works it out (fleet_link.go)
	}
	if p.TodoID != "" && p.ResumeSessionID == "" && strings.TrimSpace(p.Prompt) == "" && len(p.Images) == 0 {
		cwd := res.Session.Cwd
		if cwd == "" {
			cwd = startCwd(m, p.Cwd)
		}
		h.fresh.add(m.cfg.ID, res.Session.SessionID, p.TodoID, cwd) // its first message gets the background
	}
	if p.TodoID != "" {
		// The manager links it (a session without a title is named from its prompt) and says the todo's title.
		linked, err := h.asst().TodoLink(ctx, assistantwire.TodoLinkParams{TodoID: p.TodoID, MachineID: m.cfg.ID,
			SessionID: res.Session.SessionID, Title: res.Session.Title, Cwd: res.Session.Cwd, Agent: res.Session.Agent, Prompt: nameFrom})
		if err != nil {
			h.logf("startSession: link todo %s for %s/%s failed: %v", p.TodoID, m.cfg.ID, res.Session.SessionID, err)
		} else {
			res.Session.Todo = &proto.TodoRef{ID: p.TodoID, Title: linked.Title}
		}
	}
	m.mu.Lock()
	engine := proto.EngineNames[cmp.Or(p.Host, startEngine(m.info))]
	m.mu.Unlock()
	res.LauncherNote = apierr.JoinNotes(h.requestLang(r), note, apierr.StartInstead(h.requestLang(r), engine, m.name(), res.Instead, res.Session))
	writeJSON(w, res)
	if nameAfterStart && p.TabName == "" {
		// The name takes a model call (seconds): the tab opens as agent-<n> and is renamed when it comes.
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if name := h.autoTabName(ctx, nameFrom); name != "" {
				if err := m.call(ctx, "session.rename", map[string]any{"session_id": id, "tab_name": name}, nil); err != nil {
					h.logf("tab name %s: %v", id, err)
				}
			}
		}(res.Session.SessionID)
	}
}

// startEnded tells the machine's manager how an app's start went, for launcher health (#1642,
// assistant/launcher_health_starts.go): one that failed, or one the node ran before answering that worked; the manager
// follows one answered at once (async) in the session list. A start whose outcome the hub does not know (the machine
// away, the call timed out) is not told, nor a resume on the launcher it had (which one the node ran is its).
func (h *Hub) startEnded(mid, resume string, params map[string]any, err error) {
	launcher, _ := params["launcher"].(string)
	if launcher == "" && resume != "" {
		return
	}
	r := assistantwire.StartEnded{MachineID: mid, SessionID: resume, Launcher: launcher, OK: err == nil}
	if err != nil {
		if code := apierr.NodeCode(err); code == "timeout" || code == "node_unreachable" || errors.Is(err, context.Canceled) {
			return
		}
		r.Error = err.Error()
		var re *rpc.Error
		if errors.As(err, &re) {
			// Work picker refusals happen before the launcher runs. Counting
			// them as failures could hide a working launcher from the picker.
			// So does a resume of a conversation the computer has nothing of,
			// which a manager from before that refusal counts against the
			// launcher (#10941).
			if strings.HasPrefix(re.Code, "work_") || re.Code == proto.CodeResumeUnknown {
				return
			}
			r.Code, r.Data = re.Code, re.Data
		}
	}
	h.asstOf(h.machineOwner(mid)).notify(assistantwire.EventStartEnded, r)
}

// startEngine is the host a machine's starts open in when they name none (its default engine host), as its node
// reports it; "" from a node that reports none.
func startEngine(info proto.NodeInfo) string {
	if info.EngineHosts != nil && info.EngineHosts.Default != "" {
		return info.EngineHosts.Default
	}
	return info.Engine
}

// startsAsync: a start the app asked to have answered at once is, when the node can acknowledge it before the session
// is ready (only Claude can, and not a resume).
func startsAsync(asked bool, resume, launcher, agentType string) bool {
	return asked && resume == "" && (agentType == "claude" || (agentType == "" && launcher != "agy"))
}

// startBudget is how long a start may take: one acknowledged before the session is ready a moment, one answered only
// after startup (a sync one, Codex, a compatible CLI) as long as a busy machine needs.
func startBudget(async bool) time.Duration {
	if async {
		return 15 * time.Second
	}
	return 100 * time.Second
}

// ---- WebSocket ----

type client struct {
	canonicalContracts atomic.Bool
	cancel             context.CancelFunc
	done               <-chan struct{} // the phone connection ended; pending writes become uncertain
	stopping           atomic.Bool
	dropped            atomic.Bool // closed for falling behind (dropSlow): the close runs once, not once per send that finds it full

	hub            *Hub
	conn           *websocket.Conn
	out            chan []byte
	mu             sync.Mutex
	traffic        trafficAttribution // inferred once from the upgrade; hello may refine platform
	subs           map[[2]string]bool
	fsw            map[[3]string]fsWatchReq // {machine id, cwd, asked id} -> the folders this browser has open there (fswatch.go)
	phone          bool
	phoneID        string
	phoneToolNames map[string]bool
	activeAt       time.Time
	sim            bool // a simulator registered as phone: only used when no real iPhone is connected
	// runtimeCLI: the phone's DEBUG app offers its runtime CLI (runtime_cli.go); never one of the phone's tools.
	runtimeCLI bool
	// desktop: a desktop app (the Mac app, register_desktop); never the phone (desktops.go).
	desktop bool
	// device and role: whose credential opened the connection (devicetokens.go); device is empty for the pairing token.
	device, role string
	uiConnection string // hub-owned identity for targeted extension page transport
	uiRequests   chan struct{}
	uiManagers   map[string]bool
	// pushDevice: the app's id for pushes (register_phone's / register_desktop's device_id, PushRegistrar.deviceID in
	// the app): while it is connected its app is open, and a phone draws its Live Activity itself (liveactivity.go), a
	// desktop posts its own notifications (desktopOpen). Empty for an older app.
	pushDevice string
	// delta: the app asked for sessions_delta (sessions_push.go); have is, by machine, each session it has (id -> its
	// JSON as sent), under the hub's sessionsMu.
	delta    bool
	listMode bool // sessions_list: HTTP pages plus lightweight invalidations, never a full snapshot
	have     map[string]map[string][]byte
	// urlFeatures: what the app listed in the WebSocket URL's ?features= (wants).
	urlFeatures map[string]bool
	// clean: the app closed the connection itself (gone to the background), rather than the network dropping it.
	clean           bool
	lazy            bool // asked for items without image data (subscribe with lazy_images)
	fold            bool // asked for backlogs with the agent's work folded (subscribe with fold_work)
	name            string
	since           time.Time
	meta            clientMeta // what it said about itself in client_hello (clientmeta.go)
	acceptLang      i18n.Lang  // its upgrade request's Accept-Language (locale.go)
	deviceSessionID int64      // immutable after registration in Hub.clients
}

// sendJSON queues v without ever waiting: notifyWSFrom calls it under h.mu, so a client too slow to take it is dropped
// in the background (queue), not by a close that waits on its stuck connection.
func (c *client) sendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.queue(b)
}

func (h *Hub) broadcast(v any) {
	m, _ := v.(map[string]any)
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	typ, _ := m["type"].(string)
	var elicitation bool
	if ix, ok := m["interaction"].(proto.Interaction); ok {
		elicitation = ix.Kind == proto.IxElicitation
	}
	h.broadcastTo(typ, b, elicitation)
}

// broadcastRaw sends every app the message b (of type typ).
func (h *Hub) broadcastRaw(typ string, b []byte) { h.broadcastTo(typ, b, false) }

// broadcastTo sends every app the message b (of type typ); an MCP form or URL request (elicitation) only goes to the
// apps that said they can answer one.
func (h *Hub) broadcastTo(typ string, b []byte, elicitation bool) {
	switch typ {
	case "sessions", "interaction", "interaction_resolved": // what the Fleet activity is made of
		h.live.changed()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.deletedConversationEventLocked(typ, b) {
		return
	}
	if h.events == nil {
		h.events = newEventLog()
	}
	b = h.events.add(b, elicitation, time.Now())
	// Each broadcast has one type and localizer; keep its language forms local to this call.
	localize := i18n.ExtensionEvent
	if typ == "machines" {
		localize = localizedMachineEvent
	}
	forms := map[i18n.Lang][]byte{}
	for c := range h.clients {
		if c.role == roleScoped && typ != "sessions" {
			continue
		}
		if elicitation && !c.has("elicitation") {
			continue
		}
		if typ == legacyLarkEvent && c.has(featureLarkViaMessaging) {
			continue
		}
		c.queue(localizedOnce(forms, c.lang(), b, localize))
	}
}

// legacyLarkEvent is the Lark extension's old change notice ({"type":"lark","feed_ids":…}); while the old /api/v1/lark
// is still read it comes beside the same change's "messaging" event (docs/extensions/extension-messaging.md).
const legacyLarkEvent = "lark"

// featureLarkViaMessaging: in client_hello, the app follows the "messaging" events for its Feishu screens and drops
// the legacy "lark" one, so it is not sent to it (#9591). An app says it only while it uses a messaging source, and
// says it again in a new client_hello when that changes; replayed events on resume, before its hello, still include it.
const featureLarkViaMessaging = "lark_via_messaging"

func (h *Hub) toSubscribers(machineID, sessionID string, v any) {
	h.toSubscribersWith(machineID, sessionID, "", v)
}

// toSubscribersWith is toSubscribers for the subscribers that asked for feature ("": all of them), on the WebSocket
// URL's ?features= or in client_hello. v is encoded only once some client follows the session: most streamed deltas
// are for sessions no app has open.
func (h *Hub) toSubscribersWith(machineID, sessionID, feature string, v any) {
	if h.conversationDeleted(machineID, sessionID) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.toSubscribersLocked(machineID, sessionID, feature, v)
}

func (h *Hub) toSubscribersLocked(machineID, sessionID, feature string, v any) {
	key := [2]string{machineID, sessionID}
	var b []byte
	for c := range h.clients {
		c.mu.Lock()
		sub := c.subs[key]
		c.mu.Unlock()
		if !sub || (feature != "" && !c.wants(feature)) {
			continue
		}
		if b == nil {
			var err error
			if b, err = json.Marshal(v); err != nil {
				return
			}
		}
		c.queue(b) // a full queue drops the client (it asks again from its cursor), never just this message (#3036)
	}
}

// The live per-session pushes (session.delta, session.screen, session.live from a node), typed so a push costs no map
// and no key sort. Fields are in sorted key order, the bytes the map[string]any each replaced encoded to.
type (
	deltaPush struct {
		MachineID string `json:"machine_id"`
		SessionID string `json:"session_id"`
		Text      string `json:"text"`
		Type      string `json:"type"` // "delta"
	}
	screenPush struct {
		MachineID string    `json:"machine_id"`
		SessionID string    `json:"session_id"`
		Status    string    `json:"status"`
		Text      string    `json:"text"`
		TS        time.Time `json:"ts"`
		Type      string    `json:"type"` // "screen"
	}
	livePush struct {
		Live      *proto.SessionLive `json:"live"`
		MachineID string             `json:"machine_id"`
		SessionID string             `json:"session_id"`
		Type      string             `json:"type"` // "live"
	}
)

// itemsToSubscribers fans new items out, without image data to clients that load images on demand. moreAfter marks
// a producer's capped replay batch, whose remaining items the client must fetch.
func (h *Hub) itemsToSubscribers(machineID, sessionID string, items []proto.Item, span itemSpan, moreAfter bool) {
	if h.conversationDeleted(machineID, sessionID) {
		return
	}
	if items == nil {
		items = []proto.Item{}
	}
	msg := func(items []proto.Item) []byte {
		out := span.into(map[string]any{"type": "items", "machine_id": machineID, "session_id": sessionID, "items": items})
		if moreAfter {
			out["more_after"] = true
		}
		b, _ := marshalItems(out)
		return b
	}
	// One message per language and image mode: the node's Chinese lines read in each connection's (transcript_text.go).
	type form struct {
		lang             i18n.Lang
		lazy             bool
		markdownCloseout bool
		ring             bool // draws ```persona-card rings (card_compat.go)
	}
	msgs := map[form][]byte{}
	key := [2]string{machineID, sessionID}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.mu.Lock()
		sub, wantsLazy := c.subs[key], c.lazy
		c.mu.Unlock()
		if !sub {
			continue
		}
		f := form{lang: c.lang(), lazy: wantsLazy, ring: c.has(featureCardRing), markdownCloseout: c.has(featureMarkdownCloseout)}
		b, ok := msgs[f]
		if !ok {
			its := cardCompatItems(f.lang, f.ring, f.markdownCloseout, localizedItems(f.lang, items))
			if f.lazy {
				its = proto.WithoutImageData(its)
			}
			b = msg(its)
			msgs[f] = b
		}
		c.queue(b) // not select/default: a burst's tail dropped here left a page behind with nothing to tell it (#3036)
	}
}

func (h *Hub) phoneClient(tool ...string) *client {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Only connected providers of the requested tool are eligible; prefer real devices, then recent activity.
	var best *client
	for c := range h.clients {
		select {
		case <-c.done:
			continue
		default:
		}
		if !c.phone || (len(tool) > 0 && !c.phoneToolNames[tool[0]]) {
			continue
		}
		if best == nil || (best.sim && !c.sim) || (best.sim == c.sim &&
			(c.activeAt.After(best.activeAt) || (c.activeAt.Equal(best.activeAt) && c.since.After(best.since)))) {
			best = c
		}
	}
	return best
}

func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.wsStopping {
		h.mu.Unlock()
		http.Error(w, "hub stopping", http.StatusServiceUnavailable)
		return
	}
	h.wsHandlers.Add(1)
	h.mu.Unlock()
	defer h.wsHandlers.Done()
	upgrade := &wsUpgrade{ResponseWriter: w}
	conn, err := websocket.Accept(upgrade, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	conn.SetReadLimit(64 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	who := principalOf(r.Context())
	c := &client{cancel: cancel, done: ctx.Done(), hub: h, conn: conn, out: make(chan []byte, 512), subs: map[[2]string]bool{}, since: time.Now(),
		device: who.ID, role: who.Role, delta: who.Role != roleScoped && wantsFeature(r, featureSessionsDelta), listMode: who.Role != roleScoped && wantsFeature(r, featureSessionsList), have: map[string]map[string][]byte{},
		urlFeatures: urlFeatures(r), acceptLang: i18n.FromRequest(r), meta: clientMeta{updates: make(chan heartbeatConfig, 512), done: ctx.Done()}}
	c.traffic = h.requestTrafficAttribution(r, who)
	c.uiConnection = randomHex(24)
	c.uiRequests = make(chan struct{}, 8)
	c.uiManagers = map[string]bool{}
	if c.listMode {
		h.sessionListIndex() // before registration: never seed the index under h.mu
	}
	h.mu.Lock()
	if h.wsStopping || h.isDeviceRevoked(c.device) {
		h.mu.Unlock()
		cancel()
		_ = conn.CloseNow()
		return
	}
	if h.wsConnections == nil {
		h.wsConnections = map[*client]struct{}{}
	}
	h.wsConnections[c] = struct{}{}
	h.mu.Unlock()
	var session *deviceSessionConnection
	var writerDone chan struct{}
	defer func() {
		cancel()
		_ = conn.CloseNow()
		if writerDone != nil {
			<-writerDone
		}
		session.disconnected(err)
		h.dropClient(c)
		c.mu.Lock()
		subs := c.subs
		c.subs = map[[2]string]bool{}
		c.mu.Unlock()
		for k := range subs {
			if m := h.localMachine(k[0]); m != nil {
				// Each unwatch on its own: one node slow to answer holds up neither the others nor what follows here.
				m.watcherLeft(k[1])
				go m.syncUnwatch(k[1])
			}
		}
		h.terminalDrop(c)
		h.fullDrop(c)
		h.uiConnectionGone(c)
	}()

	// Track registration too: shutdown must not miss a newly hijacked connection.
	session = h.deviceSessionConnected(r.WithContext(ctx), c)
	if session != nil {
		// Normal client cancellation is not a hub shutdown.
		session.ctx = r.Context()
		c.deviceSessionID = session.id
	}
	// The hello goes first, then (resuming) what the client missed, before any live event: under h.mu, which
	// broadcastTo holds while it numbers and sends (eventlog.go).
	hello := h.hello()
	stream, after, resuming := resumeOf(r)
	h.mu.Lock()
	// Check under the registration lock so revocation either rejects this client or sees it to close it.
	if h.wsStopping || h.isDeviceRevoked(c.device) {
		h.mu.Unlock()
		return
	}
	h.clients[c] = struct{}{}
	if c.role == roleScoped {
		c.sendJSON(hello)
	} else {
		h.helloLocked(c, hello, stream, after, resuming)
	}
	h.mu.Unlock()
	go h.fleetClientsChanged() // an app that takes elicitations (fleet_link.go)

	writerDone = make(chan struct{})
	go func() {
		defer close(writerDone)
		defer cancel()
		session.writerStopped(c.writeWS(ctx, conn, func(hb heartbeatConfig) {
			session.heartbeat(hb)
			if err := setHeartbeatKeepAlive(upgrade.conn, hb); err != nil {
				h.logf("websocket keepalive: %v", err)
			}
		}))
	}()

	c.sendJSON(map[string]any{"type": "machines", "machines": localizedMachines(c.lang(), h.machineList())})
	if c.listMode {
		h.sessionListNotice(c)
	} else {
		for _, m := range h.machinesNow() {
			if c.delta {
				h.snapshotSessions(c, m)
			} else {
				// The kept list, not decorated's copy of every row the manager listed: the archived rows told apart are
				// never shown (#6836), and stamped copies only when a row lacks the manager.
				shown := h.shown(m.cfg.ID, stamped(m.keptSessions(), h.machineOwner(m.cfg.ID)))
				c.sendJSON(map[string]any{"type": "sessions", "machine_id": m.cfg.ID, "sessions": shown})
			}
		}
	}
	h.sendPendingInteractions(c, false)
	h.sendDesktops(c)

	for {
		_, data, readErr := conn.Read(ctx)
		if readErr != nil {
			err = readErr
			s := websocket.CloseStatus(readErr)
			c.clean = s == websocket.StatusNormalClosure || s == websocket.StatusGoingAway
			return
		}
		c.recordSocketTraffic("websocket_frame", "received", int64(len(data)), 0)
		h.onClientMessage(ctx, c, data)
	}
}

func (h *Hub) onClientMessage(ctx context.Context, c *client, data []byte) {
	var msg struct {
		Type       string          `json:"type"`
		MachineID  string          `json:"machine_id"`
		SessionID  string          `json:"session_id"`
		SinceSeq   int             `json:"since_seq"`
		LazyImages bool            `json:"lazy_images"`
		FoldWork   bool            `json:"fold_work"`
		DeviceName string          `json:"device_name"`
		DeviceID   string          `json:"device_id"` // register_phone / register_desktop: the app's id for pushes
		Platform   string          `json:"platform"`
		Simulator  bool            `json:"simulator"`
		Tools      []proto.Tool    `json:"tools"`
		CallID     string          `json:"call_id"`
		Content    []proto.Content `json:"content"`
		IsError    bool            `json:"is_error"`
		Epoch      string          `json:"epoch"`
		AfterSeq   int             `json:"after_seq"`
		// terminal_subscribe, terminal_input: the grid the app drives the terminal at (docs/integrations/terminal.md)
		Control  *termCtl `json:"control"`
		Takeover bool     `json:"takeover"`
		Text     string   `json:"text"`
		// terminal_input: the wheel over a terminal the app drives, in lines (> 0 back in history, < 0 forward), over
		// the cell of its screen where the finger went down ([column, row], 0-based)
		Scroll int   `json:"scroll"`
		At     []int `json:"at"`
		// terminal_input: a tap on a terminal the app drives, at a cell of its screen ([column, row], 0-based)
		Click []int `json:"click"`
		// fs_watch: the folders of machine_id this browser has open, and whether it shows cwd's Git status (fswatch.go)
		Cwd   string   `json:"cwd"`
		Paths []string `json:"paths"`
		Git   bool     `json:"git"`
		// herdr_open, herdr_input, herdr_resize, herdr_ack, herdr_focus: the web's full herdr view (herdrclient.go)
		ID    string `json:"id"`
		Data  string `json:"data"`
		B64   string `json:"b64"`
		Cols  int    `json:"cols"`
		Rows  int    `json:"rows"`
		Bytes int64  `json:"bytes"`
		Focus bool   `json:"focus"`
		// terminal_input: one mouse event on a terminal the app drives (feature terminal_mouse, docs/integrations/terminal.md "Mouse")
		Mouse *termMouse `json:"mouse"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	if c.role == roleScoped && !scopedSocketCommand(msg.Type) {
		return
	}
	h.mu.Lock()
	c.activeAt = time.Now()
	h.mu.Unlock()
	switch msg.Type {
	case "extension_ui_stream":
		h.forwardUIStream(ctx, c, data)
	case "client_hello":
		h.onClientHello(c, data)
	case "ping":
		h.onAppPing(c, data)
	case "terminal_subscribe":
		if c.role != roleFull && paneAddress(msg.SessionID) { // a shell by pane: full devices only (panes.go)
			c.sendJSON(map[string]any{"type": "terminal", "machine_id": msg.MachineID, "session_id": msg.SessionID, "error": "forbidden: a terminal opened by pane needs a full device"})
			return
		}
		if c.role != roleFull && msg.Control != nil { // driving a terminal is typing into it: an answer-only device only watches
			msg.Control, msg.Takeover = nil, false
			c.sendTerm(termControlMessage(msg.MachineID, msg.SessionID, "unsupported", nil, i18n.T(c.lang(), "hub-http.watch_only")))
		}
		h.terminalSubscribe(ctx, c, msg.MachineID, msg.SessionID, msg.Epoch, msg.AfterSeq, msg.Control, msg.Takeover)
	case "terminal_input":
		h.terminalInput(c, msg.MachineID, msg.SessionID, msg.Text, msg.Control, msg.Scroll, msg.At)
		if len(msg.Click) == 2 {
			h.terminalClick(c, msg.MachineID, msg.SessionID, msg.Click[0], msg.Click[1])
		}
		if msg.Mouse != nil {
			h.terminalMouse(c, msg.MachineID, msg.SessionID, *msg.Mouse)
		}
	case "terminal_unsubscribe":
		h.terminalUnsubscribe(c, msg.MachineID, msg.SessionID)
	case "fs_watch": // #2439: full devices only, like listing the folders (an answer-only device cannot)
		if c.role != roleFull {
			return
		}
		h.fsWatchSet(c, msg.MachineID, cleanFsWatch(msg.Cwd, msg.Paths, msg.Git))
	case "herdr_open":
		h.herdrOpen(ctx, c, msg.MachineID, msg.ID, msg.SessionID, msg.Cols, msg.Rows, msg.Focus)
	case "herdr_input":
		h.herdrInput(c, msg.ID, msg.Data, msg.B64)
	case "herdr_resize":
		h.herdrResize(c, msg.ID, msg.Cols, msg.Rows)
	case "herdr_ack":
		h.herdrAck(c, msg.ID, msg.Bytes)
	case "herdr_close":
		h.herdrClose(c, msg.ID)
	case "herdr_focus": // a view shows another session or pane: herdr's focus moves with it (herdrclient.go)
		h.herdrFocus(c, msg.ID, msg.SessionID)
	case "subscribe":
		m := h.localMachine(msg.MachineID)
		if m == nil {
			return
		}
		if c.role == roleScoped && !h.scopedPublicSession(m.cfg.ID, msg.SessionID) {
			return
		}
		if m.conversationDeleted(msg.SessionID) {
			c.sendJSON(deletedConversationEvent(m.cfg.ID, msg.SessionID))
			return
		}
		// Kept under the computer's own id, which is what its pushes carry (a page may have asked by an absorbed one).
		key := [2]string{m.cfg.ID, msg.SessionID}
		c.mu.Lock()
		already := c.subs[key]
		c.subs[key] = true
		if msg.LazyImages {
			c.lazy = true
		}
		if msg.FoldWork {
			c.fold = true
		}
		c.mu.Unlock()
		if !already {
			m.watcherJoined(msg.SessionID)
		}
		// Not in the read loop: the node may take up to watchCallTimeout to answer the watch (watcherJoined). The
		// backlog still goes after it, so nothing falls between the two.
		go func() {
			if !already {
				m.syncWatch(ctx, msg.SessionID)
			}
			h.sendBacklog(ctx, c, m, msg.SessionID, msg.SinceSeq)
		}()
	case "unsubscribe":
		m := h.localMachine(msg.MachineID)
		if m == nil {
			return
		}
		key := [2]string{m.cfg.ID, msg.SessionID}
		c.mu.Lock()
		had := c.subs[key]
		delete(c.subs, key)
		c.mu.Unlock()
		if had {
			m.watcherLeft(msg.SessionID)
			go m.syncUnwatch(msg.SessionID)
		}
	case "register_phone":
		if c.role != roleFull { // runs tools on behalf of claude: not for an answer-only device
			return
		}
		tools, cli := splitRuntimeCLI(msg.Tools)
		if cli && len(tools) == 0 {
			// Only the runtime CLI: a simulator driven by a script (docs/operations/runtime-cli.md "Reaching a real device through the hub"). Not the
			// phone: it takes no phone_* call and changes nothing about pushes.
			h.mu.Lock()
			c.runtimeCLI, c.sim, c.name, c.since = true, msg.Simulator, msg.DeviceName, time.Now()
			h.mu.Unlock()
			h.noteClientAppSimulator(c, msg.Simulator)
			return
		}
		deviceID := msg.DeviceID
		if deviceID == "" {
			deviceID = c.device
		}
		if deviceID == "" {
			deviceID = "legacy"
		}
		h.mu.Lock()
		c.phone, c.sim, c.name, c.since, c.pushDevice = true, msg.Simulator, msg.DeviceName, time.Now(), msg.DeviceID
		c.runtimeCLI = cli
		c.phoneID = deviceID
		c.phoneToolNames = map[string]bool{}
		for _, tool := range tools {
			c.phoneToolNames[tool.Name] = true
		}
		h.mu.Unlock()
		h.noteClientAppSimulator(c, msg.Simulator)
		if strings.HasPrefix(c.device, "pair_") {
			// A pairing token is traded for a device token within seconds and the phone registers again under it:
			// calls still reach this connection, but nothing is saved under a credential about to disappear.
			return
		}
		// Its app reaches the hub: what the hub push-starts there is answered again (#7641). Off the read loop, as the
		// tools' write. By its push id, which starts are claimed under; not deviceID above, which falls back to the credential.
		h.live.phoneConnectedLater(msg.DeviceID, c.device)
		h.setPhoneTools(db.PhoneToolRegistration{DeviceID: deviceID, Platform: msg.Platform, CredentialID: c.device, Tools: tools})
	case "register_desktop":
		h.registerDesktop(c, msg.DeviceName, msg.DeviceID)
	case "unregister_desktop":
		h.unregisterDesktop(c)
	case "seen":
		if m := h.localMachine(msg.MachineID); m != nil && msg.SessionID != "" {
			h.markSeen(m.cfg.ID, msg.SessionID)
		}
	case "tool_result":
		if c.role != roleFull {
			return
		}
		h.toolResult(msg.CallID, proto.ToolResult{Content: msg.Content, IsError: msg.IsError})
	}
}

// backlogPages caps a catch-up at 4000 items; past that the last batch says more_after and the app takes the latest
// page instead (an old app would keep the hole, as before).
const backlogPages = 8

func (h *Hub) sendBacklog(ctx context.Context, c *client, m *Machine, sid string, since int) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.mu.Lock()
	lazy, fold := c.lazy, c.fold
	c.mu.Unlock()
	after := since
	for page := 0; ; page++ {
		params := map[string]any{"session_id": sid, "turns": 2, "lazy_images": lazy} // the last two exchanges, however many rows they are
		if since > 0 {
			params["after_seq"] = after
		} else if fold {
			params["fold_work"] = true
		}
		var res struct {
			Items     []proto.Item `json:"items"`
			HasMore   bool         `json:"has_more"`
			MoreAfter bool         `json:"more_after"`
			Partial   bool         `json:"partial,omitempty"`
			itemSpan
		}
		if err := m.call(ctx, "session.items", params, &res); err != nil {
			_, raw := callErrBytes(c.lang(), err)
			var failure map[string]any
			_ = json.Unmarshal(raw, &failure)
			failure["type"], failure["machine_id"], failure["session_id"] = "error", m.cfg.ID, sid
			c.sendJSON(failure)
			return
		}
		res.Items = cardCompatItems(c.lang(), c.has(featureCardRing), c.has(featureMarkdownCloseout), localizedItems(c.lang(), h.withInjected(res.Items)))
		if lazy {
			res.Items = proto.WithoutImageData(res.Items)
		}
		out := res.itemSpan.into(map[string]any{"type": "items", "machine_id": m.cfg.ID, "session_id": sid, "items": res.Items, "backlog": true})
		if res.Partial {
			out["partial"] = true
		}
		if since == 0 {
			// Only a page counted back from the end knows whether older items exist. A catch-up after since_seq does
			// not: sending false there told the app the history was complete, and scrolling up stopped at its oldest.
			out["has_more"] = res.HasMore
		}
		// A catch-up continues page by page (one page used to be all: whatever came after its 500 items was lost).
		more := since > 0 && res.MoreAfter && len(res.Items) > 0
		if more && page+1 >= backlogPages {
			out["more_after"] = true
			more = false
		}
		if b, err := marshalItems(out); err == nil {
			if !m.queueConversationBacklog(sid, func() { c.queue(b) }) {
				c.sendJSON(deletedConversationEvent(m.cfg.ID, sid))
				return
			}
		}
		if !more {
			h.sendLiveText(c, m.cfg.ID, sid)
			return
		}
		after = res.Items[len(res.Items)-1].Seq
		if res.ToSeq != nil && *res.ToSeq > after {
			after = *res.ToSeq
		}
	}
}

// itemSpan is where a batch of items sits in the transcript (node: transcript.Span), passed through to the apps:
// history names the version of the file (it changes when the file is replaced and seqs start over), from_seq the seq
// the batch continues from (only for batches that continue: catch-up, live pushes, after_seq), to_seq the last line it
// accounts for. Older nodes send none of it.
type itemSpan struct {
	History string `json:"history,omitempty"`
	FromSeq *int   `json:"from_seq,omitempty"`
	ToSeq   *int   `json:"to_seq,omitempty"`
}

func (sp itemSpan) into(m map[string]any) map[string]any {
	if sp.History != "" {
		m["history"] = sp.History
	}
	if sp.FromSeq != nil {
		m["from_seq"] = *sp.FromSeq
	}
	if sp.ToSeq != nil {
		m["to_seq"] = *sp.ToSeq
	}
	return m
}
