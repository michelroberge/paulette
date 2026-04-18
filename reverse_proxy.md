The X-Forwarded-Proto is good to add (it affects the session cookie's Secure flag), but it likely won't fix the timeout on the pipeline GET. That endpoint is a simple JSON response - it should complete in milliseconds.

The more likely culprit is nginx buffering/timeout settings for the SSE connections. The app opens an SSE connection to /api/projects/:id/pipeline/watch simultaneously with the pipeline GET. If nginx buffers or mishandles the long-lived SSE connection, it can starve other requests to the same upstream.

For Nginx Proxy Manager, add this to the Advanced tab (Custom Nginx Configuration) for the proxy host:

first, try only 
proxy_set_header X-Forwarded-Proto $scheme;

else, try all:

proxy_buffering off;
proxy_cache off;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_read_timeout 86400s;
proxy_set_header Connection '';
proxy_http_version 1.1;
Key settings:

proxy_buffering off - critical for SSE, prevents nginx from holding responses
proxy_read_timeout 86400s - SSE connections are long-lived, default 60s timeout kills them
proxy_http_version 1.1 + empty Connection - required for proper SSE prox