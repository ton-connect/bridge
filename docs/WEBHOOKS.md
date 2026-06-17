# Multitenant Webhooks

Bridge supports per-wallet webhook delivery. When a message is sent via `/bridge/message`, the bridge looks up the recipient wallet's webhook configuration and sends a cryptographically signed notification to it. Per-wallet configuration can be provided inline via `WEBHOOK_CONFIG` and optionally overlaid from `WEBHOOK_CONFIG_SOURCE`.

## How It Works

```
  WEBHOOK_CONFIG={"wallets":{"WalletA":{"url":"URL_A","auth":"token_A"},"WalletB":{"url":"URL_B"}}}
         │
         ▼ (parsed at startup)
    webhooks map[string]WalletConfig

  POST /bridge/message?...&wallet=WalletA
         │
         ▼
  Service.GetWalletConfig("WalletA") → {URL_A, token_A}
         │
         ▼
  POST URL_A/<client_id> (async, non-blocking)
    Body: { topic, hash }
    Header: X-Webhook-Signature: <Ed25519 signature>
    Header: Authorization: Bearer token_A
```

1. At startup, the bridge parses the `WEBHOOK_CONFIG` JSON into an in-memory map of wallet name to webhook configuration (URL and optional auth token).
2. If `WEBHOOK_CONFIG_SOURCE` is set, the bridge loads the same JSON structure from that local path or URL, overlays it on top of the inline config, and refreshes it on the `WEBHOOK_CONFIG_REFRESH_INTERVAL` ticker.
3. When a message is sent with both `wallet` and `topic` query parameters, the bridge looks up the config for that wallet.
4. If found, the bridge sends a signed POST request asynchronously. If the wallet has an `auth` token, it is attached as a `Bearer` token in the `Authorization` header. Unknown wallets or missing `wallet` parameter are silently skipped.
5. The outgoing JSON payload uses `topic` from the `/bridge/message` query parameter and `hash` as the raw request body.

## Webhook Payload

```json
{
  "topic": "sendTransaction",
  "hash": "base64-encoded-message"
}
```

## Webhook Server Implementation Guide

Your webhook server receives POST requests from the bridge whenever a message is sent to your wallet. Each request is cryptographically signed so you can verify it came from a trusted bridge instance.

### What the bridge sends

```
POST <your-webhook-url>/<client_id>
Content-Type: application/json
X-Webhook-Signature: <base64-encoded signature>
Authorization: Bearer <token>          (only when wallet has "auth" configured)

{"topic":"...","hash":"..."}
```

The `X-Webhook-Signature` header contains a **base64-encoded Ed25519** signature computed over the raw JSON request body.

When a wallet's `auth` field is set in `WEBHOOK_CONFIG`, the outgoing webhook request includes an `Authorization: Bearer <token>` header. This lets the receiving server authenticate that the request came from an authorized bridge instance before even checking the cryptographic signature. Each wallet can have its own token.

### Step-by-step: handling and validating incoming webhooks

#### 1. Fetch the bridge's public key (once)

Make a GET request to the bridge's public key endpoint. Cache the result — the key only changes when the bridge restarts without a persistent key file.

```
GET https://<bridge-host>/bridge/webhook/public-key
```

Response is a PEM-encoded Ed25519 public key:

```
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA...
-----END PUBLIC KEY-----
```

Parse the PEM into an Ed25519 public key object in your language of choice and store it in memory.

#### 2. Receive the webhook POST

On each incoming request:

1. Read the **raw request body** as bytes. Do not parse or re-serialize it before verification — the signature is computed over the exact bytes the bridge sent.
2. Extract the `X-Webhook-Signature` header value.
3. If the header is missing, reject the request (the bridge always signs when a private key is available).

#### 3. Verify the signature

1. **Base64-decode** the `X-Webhook-Signature` header value (standard base64, not URL-safe).
2. **Ed25519 verify** the raw request body bytes against the decoded signature using the public key fetched in step 1.
3. If verification fails, reject the request with `401` or `403`.

#### 4. Process the payload

After successful verification, parse the JSON body:

```json
{
  "topic": "sendTransaction",
  "hash": "base64-encoded-message"
}
```

| Field | Description |
|-------|-------------|
| `topic` | Value of the `/bridge/message` `topic` query parameter |
| `hash` | Raw `/bridge/message` request body. In typical TON Connect flows this is the base64-encoded encrypted message |

Return `200 OK` to acknowledge receipt. Any non-200 response is logged as a delivery failure by the bridge.

### Complete examples

#### Go

```go
package main

import (
    "crypto/ed25519"
    "crypto/x509"
    "encoding/base64"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "io"
    "log"
    "net/http"
)

var bridgePublicKey ed25519.PublicKey

// fetchPublicKey fetches and parses the bridge's Ed25519 public key.
// Call once at startup; re-fetch if the bridge rotates its key.
func fetchPublicKey(bridgeURL string) error {
    resp, err := http.Get(bridgeURL + "/bridge/webhook/public-key")
    if err != nil {
        return fmt.Errorf("fetch public key: %w", err)
    }
    defer resp.Body.Close()

    pemBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("read public key: %w", err)
    }

    block, _ := pem.Decode(pemBytes)
    if block == nil {
        return fmt.Errorf("no PEM block found")
    }

    pub, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return fmt.Errorf("parse public key: %w", err)
    }

    edPub, ok := pub.(ed25519.PublicKey)
    if !ok {
        return fmt.Errorf("not an Ed25519 public key")
    }

    bridgePublicKey = edPub
    return nil
}

// verifySignature checks the Ed25519 signature over the raw body.
func verifySignature(body []byte, signatureB64 string) bool {
    sig, err := base64.StdEncoding.DecodeString(signatureB64)
    if err != nil {
        return false
    }
    return ed25519.Verify(bridgePublicKey, body, sig)
}

type WebhookPayload struct {
    Topic string `json:"topic"`
    Hash  string `json:"hash"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    // Step 2: read raw body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // Step 3: verify signature
    sig := r.Header.Get("X-Webhook-Signature")
    if sig == "" {
        http.Error(w, "missing signature", http.StatusUnauthorized)
        return
    }

    if !verifySignature(body, sig) {
        http.Error(w, "invalid signature", http.StatusForbidden)
        return
    }

    // Step 4: process payload
    var payload WebhookPayload
    if err := json.Unmarshal(body, &payload); err != nil {
        http.Error(w, "bad payload", http.StatusBadRequest)
        return
    }

    log.Printf("Webhook received: topic=%s hash=%s",
        payload.Topic, payload.Hash)

    w.WriteHeader(http.StatusOK)
}

func main() {
    if err := fetchPublicKey("https://bridge.example.com"); err != nil {
        log.Fatalf("Failed to fetch bridge public key: %v", err)
    }

    http.HandleFunc("/webhook", webhookHandler)
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

#### Python

```python
import base64
import json

import requests
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from cryptography.hazmat.primitives.serialization import load_pem_public_key
from flask import Flask, request, abort

app = Flask(__name__)
bridge_public_key: Ed25519PublicKey = None

def fetch_public_key(bridge_url: str):
    """Fetch and cache the bridge's Ed25519 public key."""
    global bridge_public_key
    resp = requests.get(f"{bridge_url}/bridge/webhook/public-key")
    resp.raise_for_status()
    bridge_public_key = load_pem_public_key(resp.content)

def verify_signature(body: bytes, signature_b64: str) -> bool:
    """Verify Ed25519 signature over raw body."""
    signature = base64.b64decode(signature_b64)
    try:
        bridge_public_key.verify(signature, body)
        return True
    except Exception:
        return False

@app.route("/webhook", methods=["POST"])
def webhook():
    # Step 2: read raw body
    body = request.get_data()

    # Step 3: verify signature
    sig = request.headers.get("X-Webhook-Signature")
    if not sig:
        abort(401, "Missing signature")

    if not verify_signature(body, sig):
        abort(403, "Invalid signature")

    # Step 4: process payload
    payload = json.loads(body)
    print(f"Webhook: topic={payload['topic']} hash={payload['hash']}")

    return "OK", 200

if __name__ == "__main__":
    fetch_public_key("https://bridge.example.com")
    app.run(port=8080)
```

#### JavaScript (Node.js)

```javascript
const crypto = require("crypto");
const express = require("express");

let bridgePublicKey; // KeyObject

async function fetchPublicKey(bridgeUrl) {
  const resp = await fetch(`${bridgeUrl}/bridge/webhook/public-key`);
  const pem = await resp.text();
  bridgePublicKey = crypto.createPublicKey(pem);
}

function verifySignature(body, signatureB64) {
  const sig = Buffer.from(signatureB64, "base64");
  return crypto.verify(null, body, bridgePublicKey, sig);
}

const app = express();
app.use(express.raw({ type: "application/json" }));

app.post("/webhook", (req, res) => {
  const body = req.body; // raw Buffer

  // Step 3: verify signature
  const sig = req.headers["x-webhook-signature"];
  if (!sig) return res.status(401).send("Missing signature");
  if (!verifySignature(body, sig))
    return res.status(403).send("Invalid signature");

  // Step 4: process payload
  const payload = JSON.parse(body);
  console.log(`Webhook: topic=${payload.topic} hash=${payload.hash}`);

  res.sendStatus(200);
});

fetchPublicKey("https://bridge.example.com").then(() => {
  app.listen(8080, () => console.log("Webhook server on :8080"));
});
```

### Important notes

- **Read body as raw bytes first, then verify, then parse.** If you parse JSON and re-serialize it, whitespace or key ordering may differ, causing verification to fail.
- **Cache the public key.** It doesn't change unless the bridge restarts without `WEBHOOK_PRIVATE_KEY_PATH`. You may want to re-fetch periodically or on verification failure as a fallback.
- **The bridge sends webhooks asynchronously.** There is no retry mechanism — if your server is down, the webhook is lost. The message itself is still delivered via SSE/polling.
- **Return 200 promptly.** The bridge logs non-200 responses as errors. Heavy processing should happen after responding.

## Key Management

The bridge uses an Ed25519 key pair for signing webhooks:

- **Auto-generated:** If neither `WEBHOOK_PRIVATE_KEY` nor `WEBHOOK_PRIVATE_KEY_PATH` is set, the bridge generates a new key pair at startup. The key lives only in memory and changes on restart.
- **Inline:** Set `WEBHOOK_PRIVATE_KEY` to the PEM-encoded Ed25519 private key directly (useful for environments where file mounts are inconvenient).
- **File-based:** Set `WEBHOOK_PRIVATE_KEY_PATH` to a PEM-encoded Ed25519 private key file for persistent signing across restarts.

If both `WEBHOOK_PRIVATE_KEY` and `WEBHOOK_PRIVATE_KEY_PATH` are set, the inline value takes precedence.

The public key is always available at `GET /bridge/webhook/public-key`.

## Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WEBHOOK_CONFIG` | string (JSON) | `""` | Per-wallet webhook configuration (see format below) |
| `WEBHOOK_CONFIG_SOURCE` | string | `""` | Optional local path, `file://` URL, or `http(s)://` URL that returns the same JSON object format as `WEBHOOK_CONFIG` |
| `WEBHOOK_CONFIG_REFRESH_INTERVAL` | duration | `1m` | Refresh interval for `WEBHOOK_CONFIG_SOURCE` |
| `WEBHOOK_PRIVATE_KEY` | string | - | PEM-encoded Ed25519 private key (inline). Takes precedence over `WEBHOOK_PRIVATE_KEY_PATH` |
| `WEBHOOK_PRIVATE_KEY_PATH` | string | - | Path to Ed25519 private key PEM file. If neither this nor `WEBHOOK_PRIVATE_KEY` is set, a key is generated at startup |

### `WEBHOOK_CONFIG` format

A JSON object with a `wallets` key containing a map of wallet names to configuration objects. This same format is used for both `WEBHOOK_CONFIG` and the content loaded from `WEBHOOK_CONFIG_SOURCE`:

```bash
WEBHOOK_CONFIG='{
  "wallets": {
    "testwallet": {
      "url": "https://testwallet.example.com/webhook",
      "auth": "secret-token-for-testwallet"
    },
    "otherwallet": {
      "url": "https://other.example.com/hook"
    }
  }
}'
```

| Field | Required | Description |
|-------|----------|-------------|
| `url` | yes | Base webhook endpoint URL. The bridge appends `/<client_id>` to this URL when sending |
| `auth` | no | Bearer token sent in the `Authorization` header for this wallet |

An empty string (or unset) means no inline webhooks are configured. Invalid JSON in either source will cause a startup error on the initial load. When both inline and source-backed config are present, source entries override inline entries with the same wallet key.

## API Endpoints

### `GET /bridge/webhook/public-key`

Returns the PEM-encoded Ed25519 public key used to sign webhooks.

**Response:** `200 OK` with `application/x-pem-file` content type, or `404` if no key is available.

### `POST /bridge/message` (updated)

New optional query parameter:

| Parameter | Description |
|-----------|-------------|
| `wallet`  | Wallet name (key from `WEBHOOK_CONFIG`). If present and the wallet has a registered webhook config, a signed notification is sent when `topic` is also present |

## Migration from Global Webhooks

The previous `WEBHOOK_URL` environment variable (comma-separated list of global endpoints) has been removed. To migrate:

1. Set `WEBHOOK_CONFIG` with a JSON map of wallet names to their webhook configuration.
2. Optionally set `WEBHOOK_PRIVATE_KEY_PATH` for persistent signing keys.
3. Remove `WEBHOOK_URL` from your environment.
4. Ensure callers of `/bridge/message` include the `wallet` query parameter.
5. Webhook recipients should verify signatures using the public key from `/bridge/webhook/public-key`.

Both `bridge` and `bridge3` binaries support this webhook flow.
