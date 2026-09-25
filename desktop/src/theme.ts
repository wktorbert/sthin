// Follows the system appearance: Dracula when dark, Alucard when light.
export function followSystemTheme(): void {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => document.documentElement.classList.toggle("dark", media.matches);
  apply();
  media.addEventListener("change", apply);
}
