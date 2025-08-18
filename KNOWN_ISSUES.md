# Known Issues

## Nginx Buffering Issues with Server-Sent Events

If you're experiencing issues with Server-Sent Events (SSE) being buffered when using nginx, this might be due to nginx buffering responses even with `X-Accel-Buffering: no` header. This commonly occurs when you have nginx in front of nginx - the internal nginx may "eat" the header and the external nginx will buffer the response.

To resolve this, ensure you're passing these headers:
```
Content-Type: text/event-stream
Cache-Control: no-cache
X-Accel-Buffering: no
```
