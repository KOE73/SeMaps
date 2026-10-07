/** A short-lived message at the bottom of the window, over whatever mode is shown. */
export function toast(message: string | Node): void {
  const box = document.createElement("div");
  if (typeof message === "string") box.textContent = message;
  else box.append(message);
  box.setAttribute("role", "status");
  Object.assign(box.style, {
    position: "fixed",
    left: "50%",
    bottom: "24px",
    transform: "translateX(-50%)",
    padding: "8px 14px",
    background: "#222",
    color: "#fff",
    borderRadius: "6px",
    font: "13px sans-serif",
    zIndex: "10000",
    maxWidth: "80vw",
  });
  document.body.append(box);
  setTimeout(() => box.remove(), box.querySelector("a") ? 10000 : 2600);
}
