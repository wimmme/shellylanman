// CSV text for the table and chart exports (ExportCSVAction, TimeChartsExporter).
// A value that contains the separator, a quote or a line break is quoted
// (the original writes values as they are — FEATURE_PARITY O29).

export function csvValue(v: string, sep: string): string {
  return v.includes(sep) || /["\r\n]/.test(v) ? '"' + v.replace(/"/g, '""') + '"' : v;
}

export function toCSV(header: string[], rows: string[][], sep: string): string {
  const s = sep || ',';
  return [header, ...rows].map((r) => r.map((v) => csvValue(v, s)).join(s)).join('\r\n') + '\r\n';
}

/** Offer text as a file download. */
export function download(name: string, text: string, type = 'text/csv'): void {
  const url = URL.createObjectURL(new Blob([text], { type: type + ';charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
