/** Keep the preferred fraction even while a narrow panel temporarily clamps it. */
export function styleListFraction(preferred: number, available: number): number {
  const ratio = Math.min(0.85, Math.max(0.15, preferred));
  if (available < 400) return ratio;
  return Math.min(1 - 220 / available, Math.max(180 / available, ratio));
}
