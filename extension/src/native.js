// Talks to the native messaging host (com.peregrine.host), which forwards to the app.

import { HOST_NAME } from './defaults.js';

let nextId = 1;

export class NativeError extends Error {
  constructor(code, message) {
    super(message || code);
    this.code = code;
  }
}

/**
 * Send one request. Resolves with `result`, rejects with NativeError:
 *   host_unavailable – host not installed / crashed / rejected us
 *   app_unavailable  – host ran but the app is not running and could not be started
 *   timeout, or any error code the app returned.
 */
export async function callNative(method, params, { timeoutMs = 25000 } = {}) {
  const id = nextId++;
  const msg = { id, method };
  if (params !== undefined) msg.params = params;
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new NativeError('timeout', 'Peregrine did not answer in time')), timeoutMs);
  });
  try {
    const reply = await Promise.race([
      chrome.runtime.sendNativeMessage(HOST_NAME, msg).catch((e) => {
        throw new NativeError('host_unavailable', e && e.message ? e.message : String(e));
      }),
      timeout,
    ]);
    if (!reply || typeof reply !== 'object') throw new NativeError('host_unavailable', 'empty reply');
    if (reply.error) throw new NativeError(reply.error.code || 'error', reply.error.message);
    return reply.result;
  } finally {
    clearTimeout(timer);
  }
}

/** True for errors where the browser should keep/redo the download itself. */
export function isUnavailable(err) {
  return err instanceof NativeError && ['host_unavailable', 'app_unavailable', 'timeout'].includes(err.code);
}
