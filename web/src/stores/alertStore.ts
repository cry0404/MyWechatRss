import { create } from "zustand";

interface AlertState {
  message: string | null;
  tone: "success" | "error" | "info";
  show: (message: string, tone?: "success" | "error" | "info") => void;
  hide: () => void;
}

export const useAlertStore = create<AlertState>((set) => ({
  message: null,
  tone: "error",
  show: (message, tone = "error") => set({ message, tone }),
  hide: () => set({ message: null }),
}));
