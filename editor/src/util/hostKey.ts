/** The host places this bearer key in the HTML it serves at /app/. */
export function hostWriteHeaders(): Record<string, string> {
  const key = document.querySelector<HTMLMetaElement>('meta[name="semaps-key"]')?.content;
  return key ? { Authorization: `Bearer ${key}` } : {};
}
