# UVB-76 Authentication Architecture

## Overview

UVB-76 uses session-based authentication to protect API endpoints, replacing browser-facing HTTP Basic Auth with an in-app login form.

## Design Principles

1. **No WWW-Authenticate header** - Browser requests must NEVER trigger the native HTTP Basic Auth prompt
2. **UI-first login** - Users see the polished app login form, not a browser dialog
3. **JSON 401 for APIs** - Unauthenticated API calls return `{ "error": "authentication_required" }`
4. **Session cookies** - Authentication state is maintained via HttpOnly cookies

## Authentication Flow

```
Browser → GET /                  → 200 (HTML app shell)
Browser → POST /api/v1/auth/login → Session cookie set
Browser → GET /api/v1/targets   → 200 (authenticated)
                                  + Cookie: uvb76_session=<token>
```

### Login Flow

1. User enters credentials in the login form
2. Frontend POSTs to `/api/v1/auth/login` with JSON body
3. Server validates credentials against stored password hash
4. Server generates random session token (base64-encoded)
5. Server stores token in memory session store
6. Server sets `uvb76_session` HttpOnly cookie
7. Frontend receives success response and shows dashboard

### Protected API Flow

1. Client makes request to protected endpoint
2. Session middleware extracts `uvb76_session` cookie
3. Middleware validates token against session store
4. If valid, request proceeds to handler
5. If invalid/missing, returns JSON 401 without WWW-Authenticate

## API Endpoints

### Public Endpoints (No Auth Required)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/healthz` | GET | Health check |
| `/api/v1/auth/login` | POST | Login, creates session |
| `/api/v1/auth/logout` | POST | Logout, clears session |
| `/api/v1/auth/check` | GET | Check current auth state |
| `/` | GET | Admin UI (shows login form if unauthenticated) |

### Protected Endpoints (Session Auth Required)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/targets` | GET | List targets |
| `/api/v1/targets/{id}/snapshot` | GET | Target snapshot |
| `/api/v1/targets/{id}/latency` | GET | Latency summary |
| `/api/v1/targets/{id}/latency/samples` | GET | Latency samples |
| `/api/v1/latency` | GET | All latency summaries |

## Session Cookie Properties

- **Name**: `uvb76_session`
- **HttpOnly**: true (prevents JavaScript access)
- **SameSite**: Lax (CSRF protection)
- **Secure**: true in production, false in dev mode
- **MaxAge**: 86400 seconds (24 hours)

## Error Responses

### Unauthenticated API Response

```json
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": "authentication_required"
}
```

Note: No `WWW-Authenticate` header is included.

### Login Failure Response

```json
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "success": false,
  "error": "invalid_credentials"
}
```

## Common Mistakes (How to Avoid Regression)

### DO NOT add WWW-Authenticate to UI routes

The admin UI routes must NOT use Basic Auth middleware or emit WWW-Authenticate headers:

```go
// WRONG - triggers browser auth prompt
router.Use(auth.BasicAuthMiddleware(...))
```

```go
// CORRECT - serves HTML unconditionally
router.Handle("/", http.HandlerFunc(s.handleAdmin))
```

### DO NOT add WWW-Authenticate to JSON errors

When returning 401 for unauthenticated API requests:

```go
// WRONG - RFC violation + browser prompt
w.Header().Set("WWW-Authenticate", "Basic realm=\"uvb76\"")
auth.JSONError(w, "authentication_required", 401)
```

```go
// CORRECT - JSON only, no browser prompt
auth.JSONError(w, "authentication_required", 401)
```

### DO verify no WWW-Authenticate in responses

Run the verification script:

```bash
# Test unauthenticated UI request
curl -k -D - https://127.0.0.1:8443/ | grep -i www-authenticate

# Should output nothing

# Test unauthenticated API request
curl -k -D - https://127.0.0.1:8443/api/v1/targets | grep -i www-authenticate

# Should output nothing
```

## Session Store

Sessions are stored in-memory with expiration. Each session contains:

- `Username`: The authenticated user
- `CreatedAt`: Session creation timestamp
- `ExpiresAt`: Session expiration timestamp (24 hours from creation)

## Production Considerations

- Session secret key should come from environment variable
- Consider persistent session storage for multi-instance deployments
- Token rotation on login is optional but recommended
