# Multitenant Webhooks

Bridge v3 supports per-wallet webhook delivery. When a message is sent via `/bridge/message`, the bridge looks up the recipient wallet's webhook URL and sends a cryptographically signed notification to it. Each wallet registers its own webhook endpoint in the [TON Connect wallet list](https://github.com/ton-connect/wallets-list), enabling independent notification routing without centralized configuration.

## How It Works

```
                     wallets-v2.json
                    ┌───────────────────┐
                    │ Wallet A → URL A  │
  startup/refresh → │ Wallet B → URL B  │ ← cached in memory
                    │ Wallet C → URL C  │
                    └───────────────────┘

  POST /bridge/message?...&wallet=WalletA
         │
         ▼
  Service.GetWebhookURL("WalletA") → URL A
         │
         ▼
  POST URL A (async, non-blocking)
    Body: { client_id, to, message, trace_id }
    Header: X-Webhook-Signature: <RSA-SHA256 signature>
```

1. At startup, the bridge fetches the wallet list from `WALLET_LIST_URL` and builds an in-memory map of `app_name` to webhook URL (only SSE-type bridges with a non-empty `webhook` field).
2. The wallet list is re-fetched periodically (configurable via `WALLET_LIST_REFRESH_INTERVAL`).
3. When a message is sent with a `wallet` query parameter, the bridge looks up the webhook URL for that wallet.
4. If found, the bridge sends a signed POST request asynchronously. Unknown wallets or missing `wallet` parameter are silently skipped.

## Webhook Payload

```json
{
  "client_id": "sender-address",
  "to": "recipient-address",
  "message": "base64-encoded-message",
  "trace_id": "trace-123"
}
```

## Webhook Server Implementation Guide

Your webhook server receives POST requests from the bridge whenever a message is sent to your wallet. Each request is cryptographically signed so you can verify it came from a trusted bridge instance.

### What the bridge sends

```
POST <your-webhook-url>
Content-Type: application/json
X-Webhook-Signature: <base64-encoded signature>

{"client_id":"...","to":"...","message":"...","trace_id":"..."}
```

The `X-Webhook-Signature` header contains a **base64-encoded RSA-PKCS1v15-SHA256** signature computed over the raw JSON request body.

### Step-by-step: handling and validating incoming webhooks

#### 1. Fetch the bridge's public key (once)

Make a GET request to the bridge's public key endpoint. Cache the result — the key only changes when the bridge restarts without a persistent key file.

```
GET https://<bridge-host>/bridge/webhook/public-key
```

Response is a PEM-encoded RSA public key:

```
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A...
-----END PUBLIC KEY-----
```

Parse the PEM into an RSA public key object in your language of choice and store it in memory.

#### 2. Receive the webhook POST

On each incoming request:

1. Read the **raw request body** as bytes. Do not parse or re-serialize it before verification — the signature is computed over the exact bytes the bridge sent.
2. Extract the `X-Webhook-Signature` header value.
3. If the header is missing, reject the request (the bridge always signs when a private key is available).

#### 3. Verify the signature

1. **Base64-decode** the `X-Webhook-Signature` header value (standard base64, not URL-safe).
2. **SHA-256 hash** the raw request body bytes.
3. **RSA PKCS1v15 verify** the hash against the decoded signature using the public key fetched in step 1.
4. If verification fails, reject the request with `401` or `403`.

#### 4. Process the payload

After successful verification, parse the JSON body:

```json
{
  "client_id": "sender-address",
  "to": "recipient-address",
  "message": "base64-encoded-message",
  "trace_id": "trace-123"
}
```

| Field | Description |
|-------|-------------|
| `client_id` | Address of the client that sent the message |
| `to` | Address of the intended recipient |
| `message` | The message content (base64-encoded) |
| `trace_id` | Trace identifier for request correlation |

Return `200 OK` to acknowledge receipt. Any non-200 response is logged as a delivery failure by the bridge.

### Complete examples

#### Go

```go
package main

import (
    "crypto"
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "io"
    "log"
    "net/http"
)

var bridgePublicKey *rsa.PublicKey

// fetchPublicKey fetches and parses the bridge's RSA public key.
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

    rsaPub, ok := pub.(*rsa.PublicKey)
    if !ok {
        return fmt.Errorf("not an RSA public key")
    }

    bridgePublicKey = rsaPub
    return nil
}

// verifySignature checks the RSA-SHA256 signature over the raw body.
func verifySignature(body []byte, signatureB64 string) error {
    sig, err := base64.StdEncoding.DecodeString(signatureB64)
    if err != nil {
        return fmt.Errorf("decode signature: %w", err)
    }

    hash := sha256.Sum256(body)
    return rsa.VerifyPKCS1v15(bridgePublicKey, crypto.SHA256, hash[:], sig)
}

type WebhookPayload struct {
    ClientID string `json:"client_id"`
    To       string `json:"to"`
    Message  string `json:"message"`
    TraceID  string `json:"trace_id"`
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

    if err := verifySignature(body, sig); err != nil {
        http.Error(w, "invalid signature", http.StatusForbidden)
        return
    }

    // Step 4: process payload
    var payload WebhookPayload
    if err := json.Unmarshal(body, &payload); err != nil {
        http.Error(w, "bad payload", http.StatusBadRequest)
        return
    }

    log.Printf("Webhook received: from=%s to=%s trace=%s",
        payload.ClientID, payload.To, payload.TraceID)

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
import hashlib
import json

import requests
from cryptography.hazmat.primitives.asymmetric import padding, utils
from cryptography.hazmat.primitives.serialization import load_pem_public_key
from flask import Flask, request, abort

app = Flask(__name__)
bridge_public_key = None

def fetch_public_key(bridge_url: str):
    """Fetch and cache the bridge's RSA public key."""
    global bridge_public_key
    resp = requests.get(f"{bridge_url}/bridge/webhook/public-key")
    resp.raise_for_status()
    bridge_public_key = load_pem_public_key(resp.content)

def verify_signature(body: bytes, signature_b64: str) -> bool:
    """Verify RSA-PKCS1v15-SHA256 signature over raw body."""
    signature = base64.b64decode(signature_b64)
    digest = hashlib.sha256(body).digest()
    try:
        bridge_public_key.verify(
            signature,
            digest,
            padding.PKCS1v15(),
            utils.Prehashed(hashlib.sha256),
        )
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
    print(f"Webhook: from={payload['client_id']} to={payload['to']}")

    return "OK", 200

if __name__ == "__main__":
    fetch_public_key("https://bridge.example.com")
    app.run(port=8080)
```

#### JavaScript (Node.js)

```javascript
const crypto = require("crypto");
const express = require("express");

let bridgePublicKey;

async function fetchPublicKey(bridgeUrl) {
  const resp = await fetch(`${bridgeUrl}/bridge/webhook/public-key`);
  bridgePublicKey = await resp.text();
}

function verifySignature(body, signatureB64) {
  const verifier = crypto.createVerify("SHA256");
  verifier.update(body);
  return verifier.verify(bridgePublicKey, signatureB64, "base64");
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
  console.log(`Webhook: from=${payload.client_id} to=${payload.to}`);

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

The bridge uses an RSA-2048 key pair for signing webhooks:

- **Auto-generated:** If `WEBHOOK_PRIVATE_KEY_PATH` is not set, the bridge generates a new key pair at startup. The key lives only in memory and changes on restart.
- **File-based:** Set `WEBHOOK_PRIVATE_KEY_PATH` to a PEM-encoded RSA private key file for persistent signing across restarts.

The public key is always available at `GET /bridge/webhook/public-key`.

## Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WALLET_LIST_URL` | string | `https://raw.githubusercontent.com/ton-connect/wallets-list/refs/heads/main/wallets-v2.json` | URL to fetch wallet webhook endpoints from |
| `WALLET_LIST_REFRESH_INTERVAL` | int | `3600` | How often to re-fetch the wallet list (seconds). `0` disables periodic refresh |
| `WEBHOOK_PRIVATE_KEY_PATH` | string | - | Path to RSA private key PEM file. If unset, a 2048-bit key is generated at startup |

## Wallet List Format

The bridge reads the standard [wallets-v2.json](https://github.com/ton-connect/wallets-list) format. Only entries with an SSE bridge type and a non-empty `webhook` field are used:

```json
[
  {
    "app_name": "testwallet",
    "bridge": [
      {
        "type": "sse",
        "url": "https://bridge.example.com/bridge",
        "webhook": "https://testwallet.example.com/webhook"
      }
    ]
  }
]
```

## API Endpoints

### `GET /bridge/webhook/public-key`

Returns the PEM-encoded RSA public key used to sign webhooks.

**Response:** `200 OK` with `application/x-pem-file` content type, or `404` if no key is available.

### `POST /bridge/message` (updated)

New optional query parameter:

| Parameter | Description |
|-----------|-------------|
| `wallet`  | Wallet `app_name` from the wallet list. If present and the wallet has a registered webhook URL, a signed notification is sent |

## Migration from Global Webhooks

The previous `WEBHOOK_URL` environment variable (comma-separated list of global endpoints) has been removed. To migrate:

1. Set `WALLET_LIST_URL` (or use the default).
2. Optionally set `WEBHOOK_PRIVATE_KEY_PATH` for persistent signing keys.
3. Remove `WEBHOOK_URL` from your environment.
4. Ensure callers of `/bridge/message` include the `wallet` query parameter.
5. Webhook recipients should verify signatures using the public key from `/bridge/webhook/public-key`.

**Note:** Bridge v1 no longer sends webhooks. Use bridge v3 for webhook support.
