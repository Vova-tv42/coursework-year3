import { useEffect, useState } from "react";

import { ThemeContext, type Theme } from "@/lib/hooks";

type ThemeProviderProps = {
  children: React.ReactNode;
  storageKey?: string;
};

function readStoredTheme(storageKey: string): Theme | null {
  const storedTheme = localStorage.getItem(storageKey);
  const isKnownTheme = storedTheme === "light" || storedTheme === "dark";
  return isKnownTheme ? storedTheme : null;
}

function getSystemTheme(): Theme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function ThemeProvider({
  children,
  storageKey = "vite-ui-theme",
}: ThemeProviderProps) {
  const [theme, setTheme] = useState<Theme>(
    () => readStoredTheme(storageKey) ?? getSystemTheme(),
  );

  useEffect(() => {
    const root = window.document.documentElement;
    root.classList.remove("light", "dark");
    root.classList.add(theme);
  }, [theme]);

  const value = {
    theme,
    setTheme: (theme: Theme) => {
      localStorage.setItem(storageKey, theme);
      setTheme(theme);
    },
  };

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}
