# Known issues

## Buffering

Some proxies may have default buffering settings that can be overridden, meaning the proxy may also modify headers.

- [Cloudflare](https://community.cloudflare.com/t/using-server-sent-events-sse-with-cloudflare-proxy/656279) and other CDNs may also utilize this header to optimize performance. 

- Some proxies, like [Nginx](https://nginx.org/en/docs/http/ngx_http_proxy_module.html), may have default buffering settings that can be overridden with this header. 


To resolve this, ensure you're passing these headers:
```
Content-Type: text/event-stream
Cache-Control: private, no-cache, no-transform
X-Accel-Buffering: no
```
