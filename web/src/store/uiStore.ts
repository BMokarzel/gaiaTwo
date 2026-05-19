// UI store global (Zustand). Mantém estado leve de chrome:
// tema (dark/light) e o rail esquerdo expandido/colapsado.
// Estado de dados (nodes/flow) fica em react-query, não aqui.

import { create } from "zustand";

type Theme = "dark" | "light";

interface UIState {
  theme: Theme;
  toggleTheme: () => void;
  setTheme: (t: Theme) => void;
}

const THEME_KEY = "costengine.theme";

function initialTheme(): Theme {
  if (typeof window === "undefined") return "dark";
  const saved = window.localStorage.getItem(THEME_KEY);
  if (saved === "dark" || saved === "light") return saved;
  return "dark";
}

export const useUIStore = create<UIState>((set) => ({
  theme: initialTheme(),
  toggleTheme: () =>
    set((s) => {
      const next: Theme = s.theme === "dark" ? "light" : "dark";
      if (typeof window !== "undefined") {
        window.localStorage.setItem(THEME_KEY, next);
      }
      return { theme: next };
    }),
  setTheme: (t) =>
    set(() => {
      if (typeof window !== "undefined") {
        window.localStorage.setItem(THEME_KEY, t);
      }
      return { theme: t };
    }),
}));
