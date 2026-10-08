// Thin fetch wrapper. The frozen error envelope is
// {"error":{"code":"<code>","message":"<text>"}} and every request must carry
// the session cookie, so `credentials: "same-origin"` is mandatory.

export class ApiError extends Error {
  code: string;
  status: number;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

type Body = Record<string, unknown> | unknown[] | undefined;

async function parse(res: Response): Promise<unknown> {
  if (res.status === 204) return undefined;
  const text = await res.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

async function toError(res: Response): Promise<ApiError> {
  let code = "internal";
  let message = res.statusText || "request failed";
  try {
    const body = (await res.json()) as {
      error?: { code?: string; message?: string };
    };
    if (body && body.error) {
      if (body.error.code) code = body.error.code;
      if (body.error.message) message = body.error.message;
    }
  } catch {
    // not JSON — keep the fallback
  }
  return new ApiError(res.status, code, message);
}

async function request(
  method: string,
  path: string,
  body?: Body | FormData,
  init?: RequestInit,
): Promise<unknown> {
  const headers = new Headers(init?.headers);
  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers.set("Content-Type", "application/json");
    payload = JSON.stringify(body);
  }
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers,
    body: payload,
    ...init,
  });
  if (!res.ok) throw await toError(res);
  return parse(res);
}

export const api = {
  get(path: string, init?: RequestInit): Promise<unknown> {
    return request("GET", path, undefined, init);
  },
  post(path: string, body?: Body): Promise<unknown> {
    return request("POST", path, body);
  },
  patch(path: string, body?: Body): Promise<unknown> {
    return request("PATCH", path, body);
  },
  put(path: string, body?: Body | FormData): Promise<unknown> {
    return request("PUT", path, body);
  },
  del(path: string): Promise<unknown> {
    return request("DELETE", path);
  },
  upload(path: string, form: FormData): Promise<unknown> {
    return request("PUT", path, form);
  },
};

// Raw POST of a WAV blob for POST /api/stt (body = raw bytes, not multipart).
export async function postAudio(
  path: string,
  blob: Blob,
  contentType: string,
): Promise<unknown> {
  const res = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": contentType },
    body: blob,
  });
  if (!res.ok) throw await toError(res);
  return parse(res);
}
