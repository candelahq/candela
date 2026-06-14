# PROXY-ERR-01: Unknown provider returns 400
POST {{CANDELA_URL}}/proxy/nonexistent/v1/chat/completions
Content-Type: application/json
{
  "model": "test",
  "messages": [{"role": "user", "content": "test"}]
}
HTTP 400
[Asserts]
body contains "unknown provider"

# PROXY-ERR-02: Invalid proxy path returns 400
POST {{CANDELA_URL}}/proxy/
Content-Type: application/json
{}
HTTP 400
