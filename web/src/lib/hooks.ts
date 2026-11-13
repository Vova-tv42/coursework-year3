import { createContext, useContext } from "react";
import { useRouteLoaderData } from "react-router";

import type { User } from "@/api/client";

export type Theme = "dark" | "light";

export type ThemeState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
};

export const ThemeContext = createContext<ThemeState | undefined>(undefined);

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined)
    throw new Error("useTheme must be used within a ThemeProvider");

  return context;
}

export const signedInRouteId = "signed-in";

export function useCurrentUser() {
  const user = useRouteLoaderData(signedInRouteId) as User | undefined;
  if (!user)
    throw new Error("useCurrentUser must be used inside the signed-in route");

  return user;
}
