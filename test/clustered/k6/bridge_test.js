import http from 'k6/http';
import { Counter, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import exec from 'k6/execution';
import sse from 'k6/x/sse';
import encoding from 'k6/encoding';

export let sse_messages = new Counter('sse_messages');
export let sse_errors = new Counter('sse_errors');
export let post_errors = new Counter('post_errors');
export let delivery_latency = new Trend('delivery_latency');
export let json_parse_errors = new Counter('json_parse_errors');
export let missing_timestamps = new Counter('missing_timestamps');

const PRE_ALLOCATED_VUS = 100
const MAX_VUS = PRE_ALLOCATED_VUS * 25

const BRIDGE_URL = __ENV.BRIDGE_URL || 'http://localhost:8081/bridge';
const TEST_DURATION = __ENV.TEST_DURATION || '2m';
const SSE_VUS = Number(__ENV.SSE_VUS || 500);
const SEND_RATE = Number(__ENV.SEND_RATE || 5000);

// Generate valid hex client IDs that the bridge expects
const ID_POOL = new SharedArray('ids', () => Array.from({length: 100}, (_, i) => {
  return i.toString(16).padStart(64, '0'); // 64-char hex strings
}));

// Test with configurable ramp up and duration
const RAMP_UP = '30s';
const HOLD = TEST_DURATION;
const RAMP_DOWN = '30s';

export const options = {
    discardResponseBodies: true,
    systemTags: ['status', 'method', 'name', 'scenario'], // Exclude 'url' to prevent metrics explosion
    thresholds: {
        http_req_failed: ['rate<0.01'],       // Less than 1% errors
        delivery_latency: ['p(95)<500'],      // p95 under 500ms
        sse_errors: ['count<10'],             // SSE should be very stable
        json_parse_errors: ['count<5'],       // Should rarely fail to parse
        missing_timestamps: ['count<100'],    // Most messages should have timestamps
    },
    scenarios: {
        sse: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: RAMP_UP, target: SSE_VUS },   // warm-up
                { duration: HOLD, target: SSE_VUS },      // steady
                { duration: RAMP_DOWN, target: 0 },       // cool-down
            ],
            gracefulRampDown: '30s',
            exec: 'sseWorker'
        },
        senders: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            timeUnit: '1s',
            preAllocatedVUs: PRE_ALLOCATED_VUS,
            maxVUs: MAX_VUS,
            stages: [
                { duration: RAMP_UP, target: SEND_RATE }, // warm-up
                { duration: HOLD, target: SEND_RATE },    // steady
                { duration: RAMP_DOWN, target: 0 },       // cool-down
            ],
            gracefulStop: '30s',
            exec: 'messageSender'
        },
    },
};

export function sseWorker() {
  // Use round-robin assignment for more predictable URLs
  const vuIndex = exec.vu.idInTest - 1;
  const groupId = Math.floor(vuIndex / 10); // 10 VUs per group
  const ids = [`client_${groupId * 3}`, `client_${groupId * 3 + 1}`, `client_${groupId * 3 + 2}`];
  const url = `${BRIDGE_URL}/events?client_id=${ids.join(',')}`;
  
  // Keep reconnecting for the test duration
  for (;;) {
    try {
      sse.open(url, { 
        headers: { Accept: 'text/event-stream' },
        tags: { name: 'SSE /events' }
      }, (c) => {
        c.on('event', (ev) => {
          if (ev.data === 'heartbeat' || !ev.data || ev.data.trim() === '') {
            return; // Skip heartbeats and empty events
          }
          try {
            const m = JSON.parse(ev.data);
            if (m.ts) {
              const latency = Date.now() - m.ts;
              delivery_latency.add(latency);
            } else {
              missing_timestamps.add(1);
              console.log('Message missing timestamp:', ev.data);
            }
          } catch(e) {
            json_parse_errors.add(1);
            console.log('JSON parse error:', e, 'data:', ev.data);
          }
          sse_messages.add(1);
        });
        c.on('error', (err) => {
          console.log('SSE error:', err);
          sse_errors.add(1);
        });
      });
    } catch (e) {
      console.log('SSE connection failed:', e);
      sse_errors.add(1);
    }
  }
}

export function messageSender() {
  // Use fixed client pairs to reduce URL variations
  const vuIndex = exec.vu.idInTest % ID_POOL.length;
  const to = ID_POOL[vuIndex];
  const from = ID_POOL[(vuIndex + 1) % ID_POOL.length];
  const topic = Math.random() < 0.5 ? 'sendTransaction' : 'signData';
  const body = encoding.b64encode(JSON.stringify({ ts: Date.now(), data: 'test_message' }));
  const url = `${BRIDGE_URL}/message?client_id=${from}&to=${to}&ttl=300&topic=${topic}`;
  
  const r = http.post(url, body, {
    headers: { 'Content-Type': 'text/plain' },
    timeout: '10s',
    tags: { name: 'POST /message' }, // Group all message requests
  });
  if (r.status !== 200) post_errors.add(1);
}
