// Downloads inside the page, with progress: a plain link to a file shows nothing in an
// installed web app (PWA) on a phone, so the firmware wizard fetches the file itself and
// then hands it to the browser as a download.

/**
 * The page-relative path of a firmware link ("fw/<token>/<file>"): it works wherever the page
 * runs (its own port, a reverse proxy, Home Assistant's ingress path), while the absolute link
 * is for the QR code. Null when the URL is not such a link.
 */
export function localPath(url: string): string | null {
  const i = url.indexOf('/fw/');
  return i < 0 ? null : url.slice(i + 1);
}

/** Bytes as "1.2 MB" (one decimal, at least 0.1 MB). */
export function megabytes(n: number): string {
  return `${Math.max(0.1, n / 1048576).toFixed(1)} MB`;
}

/** The progress text: "1.2 MB / 3.4 MB", or "1.2 MB" while the size is unknown. */
export function progressText(done: number, total: number): string {
  return total > 0 ? `${megabytes(done)} / ${megabytes(total)}` : megabytes(done);
}

/** Fetch path as a Blob, reporting (bytes so far, total or 0) as it arrives. */
export async function fetchWithProgress(path: string, onProgress: (done: number, total: number) => void): Promise<Blob> {
  const r = await fetch(path, { cache: 'no-store' });
  if (!r.ok) throw new Error(`${r.status} ${(await r.text().catch(() => '')).trim() || r.statusText}`);
  const total = Number(r.headers.get('Content-Length')) || 0;
  const type = r.headers.get('Content-Type') || 'application/octet-stream';
  if (!r.body) return r.blob();
  const reader = r.body.getReader();
  const parts: Uint8Array[] = [];
  let done = 0;
  onProgress(0, total);
  for (;;) {
    const { done: end, value } = await reader.read();
    if (end) break;
    parts.push(value);
    done += value.length;
    onProgress(done, total);
  }
  return new Blob(parts as BlobPart[], { type });
}

/** Hand a Blob to the browser as a download named name. */
export function saveBlob(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}
