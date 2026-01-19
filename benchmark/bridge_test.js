import http from 'k6/http';
import { Counter, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import exec from 'k6/execution';
import sse from 'k6/x/sse';
import encoding from 'k6/encoding';

export let sse_message_received = new Counter('sse_message_received');
export let sse_message_sent = new Counter('sse_message_sent');
export let sse_errors = new Counter('sse_errors');
export let post_errors = new Counter('post_errors');
export let delivery_latency = new Trend('delivery_latency');
export let json_parse_errors = new Counter('json_parse_errors');
export let missing_timestamps = new Counter('missing_timestamps');

const BRIDGE_URL = __ENV.BRIDGE_URL || 'http://localhost:8081/bridge';

// Auth token to bypass rate limits
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'test-token';

// 1 minutes ramp-up, 1 minutes steady, 1 minutes ramp-down
const SENDER_RAMP_UP = __ENV.SENDER_RAMP_UP || '10s';
const SENDER_HOLD = __ENV.SENDER_HOLD || '30s';
const SENDER_RAMP_DOWN = __ENV.SENDER_RAMP_DOWN || '10s';
const SENDER_DELAY = __ENV.SENDER_DELAY || '10s';

const SSE_RAMP_UP = __ENV.SSE_RAMP_UP || '10s';
const SSE_HOLD = __ENV.SSE_HOLD || '50s';
const SSE_RAMP_DOWN = __ENV.SSE_RAMP_DOWN || '10s';
const SSE_DELAY = __ENV.SSE_DELAY || '0s';

const SSE_VUS = Number(__ENV.SSE_VUS || 100);
const SEND_RATE = Number(__ENV.SEND_RATE || 1000);

const LISTENER_WRITERS_RATIO = Number(__ENV.LISTENER_WRITERS_RATIO || 3); // number of listeners per writer
const TOTAL_INSTANCES = Number(__ENV.TOTAL_INSTANCES || 1); // total number of instances in the simulation test
const CURRENT_INSTANCE = Number(__ENV.CURRENT_INSTANCE || 0); // 0 for the first instance, 1 for the second instance, etc.

const START_INDEX_OFFSET = CURRENT_INSTANCE * LISTENER_WRITERS_RATIO * SSE_VUS;
const ID_SPACE_SIZE = TOTAL_INSTANCES * LISTENER_WRITERS_RATIO * SSE_VUS - 1;

// Generate valid hex client IDs that the bridge expects
function getSSEIDs(vuIndex) {
  const startIndex = START_INDEX_OFFSET + vuIndex * LISTENER_WRITERS_RATIO;
  const ids = [];
  for (let i = 0; i < LISTENER_WRITERS_RATIO; i++) {
    ids.push([(startIndex + i).toString(16).padStart(64, '0')]);
  }
  return ids;
}

// Generate valid hex client IDs that the bridge expects
// This generates a random client ID for the sender in the ID space
function getID() {
  const targetIndex = Math.floor(Math.random() * ID_SPACE_SIZE);
  return targetIndex.toString(16).padStart(64, '0');
}

export const options = {
    discardResponseBodies: true,
    systemTags: ['status', 'method', 'name', 'scenario'], // Exclude 'url' to prevent metrics explosion
    thresholds: {
        http_req_failed: ['rate<0.0001'],
        delivery_latency: ['p(95)<2000'],
        sse_errors: ['count<10'], // SSE should be very stable
        json_parse_errors: ['count<5'], // Should rarely fail to parse
        missing_timestamps: ['count<100'], // Most messages should have timestamps
        sse_message_sent: ['count>5'],
        sse_message_received: ['count>5'],

    },
    scenarios: {
        sse: {
            executor: 'ramping-vus',
            startVUs: 0,
            startTime: SSE_DELAY,
            stages: [
                { duration: SSE_RAMP_UP, target: SSE_VUS },   // warm-up
                { duration: SSE_HOLD, target: SSE_VUS },      // steady
                { duration: SSE_RAMP_DOWN, target: 0 },       // cool-down
            ],
            gracefulRampDown: '30s',
            exec: 'sseWorker'
        },
        senders: {
            executor: 'ramping-arrival-rate',
            startRate: 0,
            startTime: SENDER_DELAY,
            timeUnit: '1s',
            preAllocatedVUs: SSE_VUS,
            stages: [
                { duration: SENDER_RAMP_UP, target: SEND_RATE }, // warm-up
                { duration: SENDER_HOLD, target: SEND_RATE },    // steady
                { duration: SENDER_RAMP_DOWN, target: 0 },       // cool-down
            ],
            gracefulStop: '30s',
            exec: 'messageSender'
        },
    },
};

export function sseWorker() {
  const ids = getSSEIDs(exec.scenario.iterationInTest);
  const url = `${BRIDGE_URL}/events?client_id=${ids.join(',')}`;
  
  // Keep reconnecting for the test duration
  for (;;) {
    try {
      sse.open(url, { 
        headers: { 
          'Accept': 'text/event-stream',
          'Authorization': 'Bearer ' + AUTH_TOKEN,
        },
        tags: { name: 'SSE /events' }
      }, (c) => {
        c.on('event', (ev) => {
          if (ev.data === 'heartbeat' || !ev.data || ev.data.trim() === '') {
            return; // Skip heartbeats and empty events
          }
          try {
            // Parse the SSE event data first
            const eventData = JSON.parse(ev.data);
            // Then decode the base64 message field
            const decoded = encoding.b64decode(eventData.message, 'std', 's');
            const m = JSON.parse(decoded);
            if (m.ts) {
              const latency = Date.now() - m.ts;
              delivery_latency.add(latency);
              sse_message_received.add(1);
            } else {
              missing_timestamps.add(1);
              console.log('Message missing timestamp:', decoded);
            }
          } catch(e) {
            json_parse_errors.add(1);
            console.log('JSON parse error:', e, 'data:', ev.data);
          }
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
  const to = getID();
  let from = getID();
  // Avoid sending message to the same client ID
  while (from === to) {
    from = getID()
  }
  
  const topic = Math.random() < 0.5 ? 'sendTransaction' : 'signData';
  const body = encoding.b64encode(JSON.stringify({ ts: Date.now(), data: `${from} ${to}` }));
  const url = `${BRIDGE_URL}/message?client_id=${from}&to=${to}&ttl=300&topic=${topic}`;
  
  const r = http.post(url, body, {
    headers: { 
      'Content-Type': 'text/plain',
      'Authorization': 'Bearer ' + AUTH_TOKEN,
    },
    timeout: '10s',
    tags: { name: 'POST /message' }, // Group all message requests
  });
  if (r.status !== 200) {
    post_errors.add(1);
  } else {
    sse_message_sent.add(1);
  }
}
