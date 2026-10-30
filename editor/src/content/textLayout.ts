/** Wrap plain text to measured width, preserving explicit line breaks. */
export function wrapText(
  value: string,
  width: number,
  maxLines: number,
  measure: (text: string) => number,
): string[] {
  const lines: string[] = [];
  for (const paragraph of value.split(/\r?\n/)) {
    let rest = paragraph.trim().replace(/\s+/g, " ");
    while (rest !== "") {
      if (lines.length === maxLines) {
        const last = lines[maxLines - 1] ?? "";
        const chars = Array.from(last);
        while (chars.length > 0 && measure(`${chars.join("").trimEnd()}…`) > width) chars.pop();
        lines[maxLines - 1] = `${chars.join("").trimEnd()}…`;
        return lines;
      }
      const chars = Array.from(rest);
      let low = 0;
      let high = chars.length;
      while (low < high) {
        const mid = Math.ceil((low + high) / 2);
        if (measure(chars.slice(0, mid).join("")) <= width) low = mid;
        else high = mid - 1;
      }
      let count = Math.max(low, 1);
      // Prefer a word boundary; split a single long identifier when necessary.
      if (count < chars.length && chars[count] !== " ") {
        const space = chars.slice(0, count).lastIndexOf(" ");
        if (space > 0) count = space;
      }
      lines.push(chars.slice(0, count).join("").trimEnd());
      rest = chars.slice(count).join("").trimStart();
    }
  }
  return lines;
}
