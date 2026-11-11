import { create } from "zustand";

interface UiState {
  token: string;
  collapsed: boolean;
  setToken: (t: string) => void;
  toggle: () => void;
}

export const useUi = create<UiState>((set) => ({
  token: sessionStorage.getItem("reforge_token") ?? "",
  collapsed: false,
  setToken: (t) => {
    if (t) sessionStorage.setItem("reforge_token", t);
    else sessionStorage.removeItem("reforge_token");
    set({ token: t });
  },
  toggle: () => set((s) => ({ collapsed: !s.collapsed })),
}));
